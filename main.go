package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/anypb"
)

type server struct {
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func (*server) receive(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer, reqChannel chan *discovery.DiscoveryRequest) {
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Error("Error while receiving message from stream", err)
		}
		if req.Node == nil || req.Node.Id == "" {
			log.Error("Node id is not provided")
		}

		select {
		case reqChannel <- req:
		case <-stream.Context().Done():
			log.Error("Stream closed")
			return
		}
	}
}

func (s *server) StreamEndpoints(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer) error {
	stop := make(chan struct{})
	reqChannel := make(chan *discovery.DiscoveryRequest, 1)
	go s.receive(stream, reqChannel)

	for {
		select {
		case _, ok := <-reqChannel:
			if !ok {
				log.Error("Error receiving request")
				return errors.New("Error receiving request")
			}
			eds, err := cache.MarshalResource(generateEDS())
			if err != nil {
				log.Error("Error while marhal resource ", err)
			}
			resp := &discovery.DiscoveryResponse{
				TypeUrl: resource.EndpointType,
				Resources: []*anypb.Any{
					{
						Value:   eds,
						TypeUrl: resource.EndpointType,
					},
				},
			}
			err = stream.Send(resp)
			if err != nil {
				log.Error("Error StreamingEndpoint ", err)
				return err
			}
		case <-stop:
			return nil
		}
	}
}

func (*server) DeltaEndpoints(stream endpointservice.EndpointDiscoveryService_DeltaEndpointsServer) error {
	log.Info("Delta service not implemented")
	return nil
}

func (*server) FetchEndpoints(ctx context.Context, req *discovery.DiscoveryRequest) (*discovery.DiscoveryResponse, error) {
	log.Info("FetchEndpoints service not implemented")
	return nil, nil
}

func generateEDS() *endpoint.ClusterLoadAssignment {
	return &endpoint.ClusterLoadAssignment{
		ClusterName: "hello-endpoint",
		Endpoints: []*endpoint.LocalityLbEndpoints{
			{
				LbEndpoints: []*endpoint.LbEndpoint{
					{
						HostIdentifier: &endpoint.LbEndpoint_Endpoint{
							Endpoint: &endpoint.Endpoint{
								Address: &core.Address{
									Address: &core.Address_SocketAddress{
										SocketAddress: &core.SocketAddress{
											Address: "10.5.1.6",
											PortSpecifier: &core.SocketAddress_PortValue{
												PortValue: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func main() {
	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 5678))
	if err != nil {
		log.Error(err)
	}

	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, &server{})

	reflection.Register(grpcServer)

	log.Infof("management server listening on %d", 5678)
	if err = grpcServer.Serve(lis); err != nil {
		log.Error(err)
	}
}
