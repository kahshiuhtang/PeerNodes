package server

// formatting and printing values to the console.
import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"peer-node/fileshare"
	pb "peer-node/fileshare"

	"google.golang.org/grpc"
)

// Used for build HTTP servers and clients.
type fileShareServerNode struct {
	pb.UnimplementedFileShareServer
	savedFiles   map[string][]*pb.FileDesc // read-only after initialized
	mu           sync.Mutex                // protects routeNotes
	currentCoins float64
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

/*
int64 file_byte_size = 1;
string file_hash_name = 2;
float currency_exchanged = 3;
string sender_id = 4;
string reciever_id = 5;
string file_ip_location = 6;
int64 seconds_timeout = 7;
*/
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

/*
string file_name_hash = 1;
string file_name = 2;
int64 file_size_bytes = 3;
string file_origin_address = 4;
string origin_user_id = 5;
float file_cost = 6;
string file_data_hash = 7;
bytes file_bytes = 8;
*/
func NotifyStoreWrapper(client pb.FileShareClient, file_name_hash string, file_name string, file_size_bytes int64, file_origin_address string, origin_user_id string, file_cost float32, file_data_hash string, file_bytes []byte) {
	var file_description = pb.FileDesc{FileNameHash: file_name_hash,
		FileName:          file_name,
		FileSizeBytes:     file_size_bytes,
		FileOriginAddress: file_origin_address,
		OriginUserId:      origin_user_id,
		FileCost:          float64(file_cost),
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
		FileCost:          float64(file_cost),
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
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *gRPCPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterFileShareServer(grpcServer, setupProducer(*gRPCPort, *httpPort))
	grpcServer.Serve(lis)
}
