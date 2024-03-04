package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	cl "peer-node/client"
	pb "peer-node/fileshare"
	sv "peer-node/server"
	"strconv"
	"strings"
)

const keyServerAddr = "serverAddr"

var (
	client             = cl.SetupClient()
	server             = sv.SetupProducer(8081, 8082) //need to setup
	file_hash_mappings = make(map[string]*pb.FileDesc)
	market_ip          = ""
	market_port        = ""
	origin_ip          = GetOutboundIP()
)

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
		// Check if the file exists in the local directory
		filePath := filepath.Join(".", filename)
		if _, err := os.Stat(filePath); err == nil {
			// Serve the file using http.ServeFile
			http.ServeFile(w, r, filePath)
			fmt.Printf("Served %s to client\n", filename)
			return
		} else if os.IsNotExist(err) {
			// File not found
			http.Error(w, "File not found", http.StatusNotFound)
			return
		} else {
			// Other error
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// Write a response indicating that no filename was found
		io.WriteString(w, "No filename found\n")
	}
}

type FileData struct {
	FileName string `json:"filename"`
	Content  []byte `json:"content"`
}

func sendFile(w http.ResponseWriter, r *http.Request) {
	// Extract filename from URL path
	filename := r.URL.Path[len("/requestFile/"):]

	// Open the file
	file, err := os.Open("./files/stored/" + filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	fileInfo, err := os.Stat("./files/stored/" + filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set content type
	contentType := "application/octet-stream"
	switch {
	case filename[len(filename)-4:] == ".txt":
		contentType = "text/plain"
	case filename[len(filename)-5:] == ".json":
		contentType = "application/json"
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

	fmt.Printf("\nFile %s sent!\n", filename)
	sv.RecordTransactionWrapper(client, fileInfo.Size(), filename, 0, "", "", "", 100)
}

func storeFile(w http.ResponseWriter, r *http.Request) {
	// Parse JSON object from request body
	var fileData FileData
	err := json.NewDecoder(r.Body).Decode(&fileData)
	if err != nil {
		http.Error(w, "Failed to parse JSON data", http.StatusBadRequest)
		return
	}

	// Create file
	file, err := os.Create("./files/stored/" + fileData.FileName)
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Write content to file
	_, err = file.Write(fileData.Content)
	if err != nil {
		http.Error(w, "Failed to write to file", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Requested client stored file %s successfully!\n", fileData.FileName)
	fmt.Printf("\nStored file %s!\n", fileData.FileName)
}

func getFileOnce(ip, port, filename string) {
	resp, err := http.Get(fmt.Sprintf("http://%s:%s/requestFile/%s", ip, port, filename))
	if err != nil {
		fmt.Printf("Error: %s\n\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: %s\n\n", resp.Status)
		return
	}

	out, err := os.Create("./files/requested/" + filename)
	if err != nil {
		return
	}
	defer out.Close()

	// Write response body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return
	}

	fmt.Printf("File %s downloaded successfully!\n\n", filename)
}

func requestStorage(ip, port, filename string) {
	// Read file content
	content, err := os.ReadFile("./files/documents/" + filename)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Create FileData struct
	fileData := FileData{
		FileName: filename,
		Content:  content,
	}

	// Marshal FileData to JSON
	jsonData, err := json.Marshal(fileData)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Send POST request to store file
	resp, err := http.Post(fmt.Sprintf("http://%s:%s/storeFile/", ip, port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println(string(body))
	fmt.Print()
}

// Ask user to enter a port and returns it
func getPort() string {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter a port number to start listening to requests: ")
	for {
		port, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			os.Exit(1)
		}
		port = strings.TrimSpace(port)

		// Validate port
		listener, err := net.Listen("tcp", ":"+port)
		if err == nil {
			defer listener.Close()
			return port
		}

		fmt.Print("Invalid port. Please enter a different port: ")
	}
}

// Start HTTP server
func startServer(port string, serverReady chan<- bool) {
	http.HandleFunc("/requestFile/", sendFile)
	http.HandleFunc("/storeFile/", storeFile)

	fmt.Printf("Listening on port %s...\n\n", port)
	serverReady <- true
	http.ListenAndServe(":"+port, nil)
}
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

// Start CLI
func startCLI() {
	fmt.Println("Dive In and Explore! Type 'help' for available commands.")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading from stdin:", err)
			continue
		}

		text = strings.TrimSpace(text)
		parts := strings.Fields(text)
		if len(parts) == 0 {
			continue
		}

		command := parts[0]
		args := parts[1:]

		switch command {
		case "get":
			if market_ip == "" || market_port == "" {
				fmt.Println("Error, market ip and market port need to be set with market command")
				continue
			}
			val, ok := file_hash_mappings["foo"]
			var best_candidate *pb.StorageIP
			if ok {
				best_candidate = cl.RequestFileFromMarket(client, val)
			} else {
				fmt.Println("Enter in file description in following format")
				fmt.Println("<FileNameHash> <FileName> <FileSizeBytes> <MaxFilePayment>")

				text, err := reader.ReadString('\n')
				if err != nil {
					fmt.Fprintln(os.Stderr, "Error reading from stdin:", err)
					continue
				}

				text = strings.TrimSpace(text)
				inputs := strings.Fields(text)
				if len(inputs) == 0 {
					continue
				}
				args := inputs[1:]
				if len(args) != 4 {
					continue
				}
				file_name_hash := args[0]
				file_byte_size, err := strconv.Atoi(args[2])
				if err != nil {
					fmt.Println("Error converting string to integer.")
					continue
				}
				file_cost, err := strconv.ParseFloat(args[3], 32)
				if err != nil {
					fmt.Println("Error converting string to float.")
					continue
				}
				byteArray := []byte("")
				fileData := pb.FileDesc{FileNameHash: file_name_hash,
					FileName:          args[1],
					FileSizeBytes:     int64(file_byte_size),
					FileOriginAddress: origin_ip.String(),
					OriginUserId:      "", //CHANGE LATER
					FileCost:          float32(file_cost),
					FileDataHash:      "",
					FileBytes:         byteArray}

				best_candidate = cl.RequestFileFromMarket(client, &fileData)
				file_hash_mappings[args[0]] = &fileData
			}
			if len(args) == 3 {
				getFileOnce(best_candidate.IpAddress, best_candidate.IpPort, args[2])
			} else {
				fmt.Println("Usage: get <ip> <port> <filename>")
				fmt.Println()
			}
		case "store":
			if len(args) == 3 {
				requestStorage(args[0], args[1], args[2])
			} else {
				fmt.Println("Usage: store <ip> <port> <filename>")
				fmt.Println()
			}
		case "unstore":
			if len(args) == 3 {
				requestStorage(args[0], args[1], args[2])
			} else {
				fmt.Println("Usage: store <ip> <port> <filename>")
				fmt.Println()
			}
		case "list":
			// TO-DO
		case "exit":
			fmt.Println("Exiting...")
			return
		case "help":
			fmt.Println("COMMANDS:")
			fmt.Println(" get <ip> <port> <filename>     Request a file")
			fmt.Println(" store <ip> <port> <filename>   Request storage of a file")
			fmt.Println(" unstore <ip> <port> <filename>   Request storage of a file")
			fmt.Println(" list                           List all files you are storing")
			fmt.Println(" exit                           Exit the program")
			fmt.Println()
		case "market":
			if len(args) == 2 {
				market_ip = args[0]
				market_port = args[1]
				fmt.Println("Updated market address")
			} else {
				fmt.Println("Usage: market <ip> <port>")
				fmt.Println()
			}
		default:
			fmt.Println("Unknown command. Type 'help' for available commands.")
			fmt.Println()
		}
	}
}

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
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server: %s\n", err)
		}
	}()
	fmt.Println("Welcome to Orcanet!")
	port := getPort()

	serverReady := make(chan bool)
	go startServer(port, serverReady)
	<-serverReady

	startCLI()
}
