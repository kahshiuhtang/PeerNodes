package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/daminals/cse416-init-repo-union-1/peernode" // Replace "your-package-path" with the actual package path
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	port = flag.Int("port", 50051, "The server port")
)

type server struct {
	pb.UnimplementedMarketServiceServer
}

type FileProducer struct {
	Link           string
	Price          float32
	PaymentAddress string
}

var fileProducerMap = make(map[string][]FileProducer)

func (s *server) AddProducer(ctx context.Context, in *pb.FileProducer) (*emptypb.Empty, error) {
	log.Printf("AddProducer for file hash: %s", in.Hash)

	// Add the producer to the map
	producer := FileProducer{
		Link:           in.Link,
		Price:          in.Price,
		PaymentAddress: in.PaymentAddress,
	}
	existingProducers, ok := fileProducerMap[in.Hash]
	if !ok {
		fileProducerMap[in.Hash] = []FileProducer{producer}
	} else {
		fileProducerMap[in.Hash] = append(existingProducers, producer)
	}

	return &emptypb.Empty{}, nil
}

func (s *server) GetProducers(ctx context.Context, in *pb.FileHash) (*pb.FileProducerList, error) {
	log.Printf("GetProducers for file hash: %s", in.Hash)

	producers, ok := fileProducerMap[in.Hash]
	if !ok {
		return &pb.FileProducerList{}, nil
	} else {
		pbProducers := make([]*pb.FileProducer, len(producers))
		for i, producer := range producers {
			pbProducers[i] = &pb.FileProducer{
				Hash:           in.Hash,
				Link:           producer.Link,
				Price:          producer.Price,
				PaymentAddress: producer.PaymentAddress,
			}
		}
		return &pb.FileProducerList{Producers: pbProducers}, nil

	}
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterMarketServiceServer(s, &server{})
	log.Println("Market Server listening on port 50051...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
