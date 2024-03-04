package server

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"peer-node/fileshare"
	pb "peer-node/fileshare"
	"strconv"
	"sync"
	"time"
)

const keyServerAddr = "serverAddr"

type fileShareServerNode struct {
	pb.UnimplementedFileShareServer
	savedFiles   map[string][]*pb.FileDesc // read-only after initialized
	mu           sync.Mutex                // protects routeNotes
	currentCoins float64
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("got /root request\n")
	io.WriteString(w, "Hello, HTTP!\n")
}

func getFile(w http.ResponseWriter, r *http.Request) {
	// Get the context from the request
	ctx := r.Context()

	// Check if the "filename" query parameter is present
	hasFilename := r.URL.Query().Has("filename")

	// Retrieve the value of the "filename" query parameter
	filename := r.URL.Query().Get("filename")

	// Print information about the request
	fmt.Printf("%s: got /file request. filename(%t)=%s\n",
		ctx.Value(keyServerAddr),
		hasFilename, filename,
	)

	// Check if the "filename" parameter is present
	if hasFilename {

		// Open the file
		filePath := filepath.Join(".", filename)
		file, err := os.Open(filePath)
		if err != nil {
			// Handle error (file not found, permission issues, etc.)
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer file.Close()

		// Check if the "chunksize" query parameter is present
		hasChunksize := r.URL.Query().Has("chunksize")

		if hasChunksize {
			// Retrieve the value of the "filename" query parameter
			chunkSizeParam := r.URL.Query().Get("chunksize")
			// Convert the value to an integer
			chunkSize, err := strconv.Atoi(chunkSizeParam)
			if err != nil {
				// Handle the error (invalid value)
				fmt.Printf("Invalid chunksize parameter value:", chunkSizeParam)
				return
			}

			fmt.Printf("chunkSize: %d\n", chunkSize)

			buffer := make([]byte, chunkSize)

			for {
				// Read 10 bytes from the file
				n, err := file.Read(buffer)
				if err != nil {
					// Check if it's the end of the file
					if err.Error() == "EOF" {
						break
					}
					// Handle other errors
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Write the 10-byte chunk to the response
				w.Write(buffer[:n])
				w.Write([]byte("\n***\n"))
			}
		} else {
			fmt.Printf("No chunk size specified\n")
			// Serve the file using http.ServeFile
			http.ServeFile(w, r, filePath)
			fmt.Printf("Served %s to client\n", filename)
			return
		}

	} else {
		// Write a response indicating that no filename was found
		io.WriteString(w, "No filename found\n")
	}
}
func sendFileToConsumer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		for k, v := range r.URL.Query() {
			fmt.Printf("%s: %s\n", k, v)
		}
		// file = r.URL.Query().Get("filename")
		w.Write([]byte("Received a GET request\n"))

	default:
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(http.StatusText(http.StatusNotImplemented)))
	}
	w.Write([]byte("Received a GET request\n"))
	filename := r.URL.Path[len("/reqFile/"):]

	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	// Set content type
	contentType := "application/octet-stream"
	switch {
	case filename[len(filename)-4:] == ".txt":
		contentType = "text/plain"
	case filename[len(filename)-5:] == ".json":
		contentType = "application/json"
		// Add more cases for other file types if needed
	}

	// Set content disposition header
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Type", contentType)

	// Copy file contents to response body
	_, err = io.Copy(w, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func runRecordTransaction(client pb.FileShareClient, transaction *pb.FileRequestTransaction) *pb.TransactionACKResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ackResponse, err := client.RecordFileRequestTransaction(ctx, transaction)
	if err != nil {
		log.Fatalf("client.RecordFileRequestTransaction failed: %v", err)
	}
	log.Printf("ACK Response: %v", ackResponse)
	return ackResponse
}

func runNotifyStore(client pb.FileShareClient, file *pb.FileDesc) *fileshare.StorageACKResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ackResponse, err := client.NotifyFileStore(ctx, file)
	if err != nil {
		log.Fatalf("client.NotifyFileStorage failed: %v", err)
	}
	log.Printf("ACK Response: %v", ackResponse)
	return ackResponse
}

func runNotifyUnstore(client pb.FileShareClient, file *pb.FileDesc) *fileshare.StorageACKResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ackResponse, err := client.NotifyFileUnstore(ctx, file)
	if err != nil {
		log.Fatalf("client.NotifyFileStorage failed: %v", err)
	}
	log.Printf("ACK Response: %v", ackResponse)
	return ackResponse
}

func RecordTransactionWrapper(client pb.FileShareClient, file_size int64, file_hash_name string, cost float64, sender_id string, receiver_id string, file_ip_location string, seconds_timeout int64) {
	var transaction = pb.FileRequestTransaction{FileByteSize: file_size,
		FileHashName:      file_hash_name,
		CurrencyExchanged: float32(cost),
		SenderId:          sender_id,
		ReceiverId:        receiver_id,
		FileIpLocation:    file_ip_location,
		SecondsTimeout:    seconds_timeout,
	}
	var ack = runRecordTransaction(client, &transaction)
	if ack.IsSuccess {
		fmt.Println("[Server]: Successfully recorded transaction in hash: %v", ack.BlockHash)
	} else {
		fmt.Println("[Server]: Unable to record transaction in blockchain")
	}
}

func NotifyStoreWrapper(client pb.FileShareClient, file_name_hash string, file_name string, file_size_bytes int64, file_origin_address string, origin_user_id string, file_cost float32, file_data_hash string, file_bytes []byte) {
	var file_description = pb.FileDesc{FileNameHash: file_name_hash,
		FileName:          file_name,
		FileSizeBytes:     file_size_bytes,
		FileOriginAddress: file_origin_address,
		OriginUserId:      origin_user_id,
		FileCost:          file_cost,
		FileDataHash:      file_data_hash,
		FileBytes:         file_bytes}
	var ack = runNotifyUnstore(client, &file_description)
	if ack.IsAcknowledged {
		fmt.Printf("[Server]: Market acknowledged stopping storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	} else {
		fmt.Printf("[Server]: Unable to notify market that we are stopping the storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	}
}
func NotifyUnstoreWrapper(client pb.FileShareClient, file_name_hash string, file_name string, file_size_bytes int64, file_origin_address string, origin_user_id string, file_cost float32, file_data_hash string, file_bytes []byte) {
	var file_description = pb.FileDesc{FileNameHash: file_name_hash,
		FileName:          file_name,
		FileSizeBytes:     file_size_bytes,
		FileOriginAddress: file_origin_address,
		OriginUserId:      origin_user_id,
		FileCost:          file_cost,
		FileDataHash:      file_data_hash,
		FileBytes:         file_bytes}
	var ack = runNotifyUnstore(client, &file_description)
	if ack.IsAcknowledged {
		fmt.Printf("[Server]: Market acknowledged stopping storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	} else {
		fmt.Printf("[Server]: Unable to notify market that we are stopping the storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	}
}

func setupProducer(gRPCPort int, httpPort int) *fileShareServerNode {
	s := &fileShareServerNode{savedFiles: make(map[string][]*pb.FileDesc)}
	// s.loadMappings(*jsonDBFile) // Have a load and save mappings
	http.HandleFunc("/file", sendFileToConsumer)
	fmt.Println("[Server]: Listening On Port" + strconv.Itoa(httpPort))
	fmt.Println("[Server]: Press CTRL + C to quit.")
	go func() {
		for {
			http.ListenAndServe(":"+strconv.Itoa(httpPort), nil)
		}
	}()
	return s
}

// Can add back in TLS later
var (
	jsonDBFile = flag.String("json_db_file", "", "A json file containing a list of features")
	gRPCPort   = flag.Int("gport", 50051, "The gRPC port for send/receive gRPC")
	httpPort   = flag.Int("hport", 50052, "The server port for listening for HTTP requests")
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/requestFile", getFile)

	ctx := context.Background()
	server := &http.Server{
		Addr:    ":3333",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error listening for server: %s\n", err)
	}
}
