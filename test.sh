#!/bin/bash

./compile.sh # build proto files

# Function to run client and capture output
run_client() {
    client_name=$1
    client_path=$2
    color_code=$3

    # Run client and capture output
    go run "$client_path" 2>&1 | sed "s/^/${esc}${color_code}${client_name}:${reset} /" &
}

# ANSI color escape codes
esc=$(printf '\033')
red="${esc}[0;31m"
green="${esc}[0;32m"
yellow="${esc}[0;33m"
reset="${esc}[0m"

# Run market client
run_client "Market" "market/mock.go" "${red}"

sleep 5

# Run producer client
run_client "Producer" "producer/cmd/producer.go" "${yellow}"

sleep 5

# Run consumer client
run_client "Consumer" "consumer/cmd/consumer.go" "${green}"

# Wait for all clients to finish
wait
