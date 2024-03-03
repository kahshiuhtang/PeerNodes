package file_hash

import (
	"bufio"
	"crypto/sha256"
	"io"
	"os"
)

type NameStore interface {
	GetFileHash(string) string
	PutFileHash(string) string
}

type FileStore interface {
	GetFile(string) ([]byte, error)
	PutFile([]byte) string
}

type NameMap struct {
	mapping map[string]string
}

type DataStore struct {
}

func (nmp *NameMap) GetFileHash(name string) string {
	return nmp.mapping[name]
}

func (nmp *NameMap) PutFileHash(name string, hash_val string) {
	nmp.mapping[name] = hash_val
}

func (ds *DataStore) GetFile(hash_val string) ([]byte, error) {
	file, err := os.Open(hash_val)
	if err != nil {
		return []byte{}, err
	}
	data, err := io.ReadAll(bufio.NewReader(file))
	if err != nil && err != io.EOF {
		return []byte{}, err
	}
	return data, nil
}

func (ds *DataStore) PutFile(data []byte) string {
	checksum := sha256.Sum256(data)
	hash_val := string(checksum[:])
	os.WriteFile(hash_val, data, 0444)

	return hash_val
}
