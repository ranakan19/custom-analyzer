package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	v1 "github.com/ranakan19/custom-analyzer/proto/schema/v1"
	"github.com/ranakan19/custom-analyzer/pkg/analyzer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	fmt.Println("Starting ApplicationSet Analyzer!")
	var err error
	address := fmt.Sprintf(":%s", "8085")
	lis, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	aa := analyzer.NewAnalyzer()
	v1.RegisterCustomAnalyzerServiceServer(grpcServer, aa.Handler)
	fmt.Printf("ApplicationSet Analyzer server listening on %s\n", address)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("Server error: %v\n", err)
		return
	}
} 