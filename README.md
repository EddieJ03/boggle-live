# Go Backend

To build Boggle Kafka server: 
- cd into `boggle-kafka` then run `go build`
- If building for Docker container, prefix above command with `GOOS=linux GOARCH=amd64 `

To build Boggle Backend server: 
- cd into `boggle-backend` then run `go build`
- If building for Docker container, prefix above command with `GOOS=linux GOARCH=amd64 `

To build docker image: `docker build -t redpanda-ws . --no-cache`
- if no file changes, can ignore adding `--no-cache`

To run docker image: `docker run --rm -p 5050:5050 -p 7001:7001 -p 9092:9092 redpanda-ws`