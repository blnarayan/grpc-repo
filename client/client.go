/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package main implements a simple gRPC client that demonstrates how to use gRPC-Go libraries
// to perform unary, client streaming, server streaming and full duplex RPCs.
//
// It interacts with the path location service whose definition can be found in pathlocation/path_location.proto.
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/examples/data"
	pb "google.golang.org/grpc/examples/place_location/placelocation"
)

var (
	tls                = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "", "The file containing the CA root cert file")
	serverAddr         = flag.String("addr", "localhost:50051", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

// printPlace gets the place for the given location.
func printPlace(client pb.PlaceLocationClient, location *pb.Location) {
	log.Printf("Getting place for location (%d, %d)", location.Latitude, location.Longitude)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	place, err := client.GetPlace(ctx, location)
	if err != nil {
		log.Fatalf("%v.GetPlace(_) = _, %v: ", client, err)
	}
	log.Println(place)
}

// printPlaces lists all the places within the given bounding Coordinates.
func printPlaces(client pb.PlaceLocationClient, rect *pb.Coordinates) {
	log.Printf("Looking for places within %v", rect)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.ListPlaces(ctx, rect)
	if err != nil {
		log.Fatalf("%v.ListPlaces(_) = _, %v", client, err)
	}
	for {
		place, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("%v.ListPlaces(_) = _, %v", client, err)
		}
		log.Printf("Place: name: %q, location:(%v, %v)", place.GetName(),
			place.GetLocation().GetLatitude(), place.GetLocation().GetLongitude())
	}
}

// runRecordPath sends a sequence of locations to server and expects to get a pathSummary from server.
func runRecordPath(client pb.PlaceLocationClient) {
	// Create a random number of random locations
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	locationCount := int(r.Int31n(100)) + 2 // Traverse at least two locations
	var locations []*pb.Location
	for i := 0; i < locationCount; i++ {
		locations = append(locations, randomLocation(r))
	}
	log.Printf("Traversing %d locations.", len(locations))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.RecordPath(ctx)
	if err != nil {
		log.Fatalf("%v.RecordPath(_) = _, %v", client, err)
	}
	for _, location := range locations {
		if err := stream.Send(location); err != nil {
			log.Fatalf("%v.Send(%v) = %v", stream, location, err)
		}
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("%v.CloseAndRecv() got error %v, want %v", stream, err, nil)
	}
	log.Printf("Path summary: %v", reply)
}

// runPathChat receives a sequence of location notes, while sending notes for various locations.
func runPathChat(client pb.PlaceLocationClient) {
	notes := []*pb.LocationNote{
		{Location: &pb.Location{Latitude: 0, Longitude: 1}, Message: "At first location"},
		{Location: &pb.Location{Latitude: 0, Longitude: 2}, Message: "At second location"},
		{Location: &pb.Location{Latitude: 0, Longitude: 3}, Message: "At third location"},
		{Location: &pb.Location{Latitude: 0, Longitude: 1}, Message: "At fourth location"},
		{Location: &pb.Location{Latitude: 0, Longitude: 2}, Message: "At fifth location"},
		{Location: &pb.Location{Latitude: 0, Longitude: 3}, Message: "At sixth location"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.PathChat(ctx)
	if err != nil {
		log.Fatalf("%v.PathChat(_) = _, %v", client, err)
	}
	waitc := make(chan struct{})
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}
			log.Printf("Got message %s at location(%d, %d)", in.Message, in.Location.Latitude, in.Location.Longitude)
		}
	}()
	for _, note := range notes {
		if err := stream.Send(note); err != nil {
			log.Fatalf("Failed to send a note: %v", err)
		}
	}
	stream.CloseSend()
	<-waitc
}

func randomLocation(r *rand.Rand) *pb.Location {
	lat := (r.Int31n(180) - 90) * 1e7
	long := (r.Int31n(360) - 180) * 1e7
	return &pb.Location{Latitude: lat, Longitude: long}
}

func main() {
	flag.Parse()
	var opts []grpc.DialOption
	if *tls {
		if *caFile == "" {
			*caFile = data.Path("x509/ca_cert.pem")
		}
		creds, err := credentials.NewClientTLSFromFile(*caFile, *serverHostOverride)
		if err != nil {
			log.Fatalf("Failed to create TLS credentials %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewPlaceLocationClient(conn)

	// Looking for a valid place
	printPlace(client, &pb.Location{Latitude: 409146138, Longitude: -746188906})

	// Place missing.
	printPlace(client, &pb.Location{Latitude: 0, Longitude: 0})

	// Looking for places between 40, -75 and 42, -73.
	printPlaces(client, &pb.Coordinates{
		Lo: &pb.Location{Latitude: 400000000, Longitude: -750000000},
		Hi: &pb.Location{Latitude: 420000000, Longitude: -730000000},
	})

	// RecordPath
	runRecordPath(client)

	// PathChat
	runPathChat(client)
}
