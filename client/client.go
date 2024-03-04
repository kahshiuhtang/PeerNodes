package client

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	pb "peer-node/fileshare"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func RequestFileFromMarket(client pb.FileShareClient, fileDesc *pb.FileDesc) *pb.StorageIP {
	log.Printf("Requesting IP For File (%s)", fileDesc.FileName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	streamOfAddresses, err := client.PlaceFileRequest(ctx, fileDesc)
	if err != nil {
		log.Fatalf("client.requestFileStorage failed: %v", err)
	}
	var possible_candidates = []*pb.StorageIP{}
	for {
		storage_ip, err := streamOfAddresses.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("client.ListFeatures failed: %v", err)
		}
		log.Printf("File %s found on address: %s for the cost of %f",
			storage_ip.FileName, storage_ip.Address, storage_ip.FileCost)
		possible_candidates = append(possible_candidates, storage_ip)
		if storage_ip.IsLastCandidate == true {
			break
		}
	}
	var best_candidate *pb.StorageIP = nil
	for _, candidate := range possible_candidates {
		if best_candidate == nil {
			best_candidate = candidate
		} else if best_candidate.FileCost < candidate.FileCost {
			best_candidate = candidate
		}
	}
	return best_candidate
}

func RequestFileFromProducer(baseURL string, filename string) bool {
	encodedParams := url.Values{}
	encodedParams.Add("filename", filename)
	queryString := encodedParams.Encode()

	// Construct the URL with the query string
	urlWithQuery := fmt.Sprintf("%s?%s", baseURL, queryString)
	resp, err := http.Get(urlWithQuery)

	if err != nil {
		log.Fatalln(err)
	}
	//We Read the response body on the line below.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	//Convert the body to type string
	sb := string(body)
	fmt.Println(sb)
	return false
}

func SetupClient() pb.FileShareClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.Dial(*marketAddr, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewFileShareClient(conn)
	return client
}

var (
	marketAddr = flag.String("addr", "localhost:50051", "The market address in the format of host:port")
)
