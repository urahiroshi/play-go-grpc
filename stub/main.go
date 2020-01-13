package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type Response struct {
	Body       string              `json:"Body"`
	StatusCode int                 `json:"StatusCode"`
	Header     map[string][]string `json:"Header"`
	Trailer    map[string][]string `json:"Trailer"`
}

var fixtureMap map[string]map[string]map[string]Response
var fixtureJson = []byte(`{
	"helloworld.Greeter": {
		"SayGoodbye": {
			"{\"hoge\":\"hogehoge\",\"fuga\":\"fugafuga\"}": {
				"Body": "{\"value\":\"hoge\"}",
				"StatusCode": 200,
				"Header": {
					"Content-Type": ["application/grpc"]
				},
				"Trailer": {
					"Grpc-Message": [""],
					"Grpc-Status": ["0"]
				}
			}
		}
	}
}`)

func newReflectionClient(ctx context.Context) *grpcreflect.Client {
	refConn, err := grpc.Dial("localhost:50052", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	stub := rpb.NewServerReflectionClient(refConn)
	return grpcreflect.NewClient(ctx, stub)
}

func getNames(u *url.URL) (string, string) {
	paths := strings.Split(u.Path, "/")
	if len(paths) < 3 {
		log.Printf("Invalid URL: %v", u.Path)
		return "", ""
	}
	serviceName := paths[1]
	methodName := paths[2]
	return serviceName, methodName
}

func createGRPCBytes(protoBytes []byte) []byte {
	length := len(protoBytes)
	return append([]byte{
		// not compress
		0,
		byte(length / (256 ^ 3)),
		byte((length % (256 ^ 3)) / (256 ^ 2)),
		byte((length % (256 ^ 2)) / 256),
		byte(length % 256),
	}, protoBytes...)
}

func writeResponse(w http.ResponseWriter, message proto.Message, res Response) {
	protoBytes, err := proto.Marshal(message)
	if err != nil {
		log.Print(err.Error())
		return
	}
	grpcBytes := createGRPCBytes(protoBytes)
	for key := range res.Trailer {
		w.Header().Add("Trailer", key)
	}
	for key, values := range res.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(res.StatusCode)
	w.Write(grpcBytes)
	for key, values := range res.Trailer {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

func main() {
	server := http2.Server{}
	l, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatal(err.Error())
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err.Error())
		}

		server.ServeConn(conn, &http2.ServeConnOpts{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// create message
				serviceName, methodName := getNames(r.URL)

				// TODO: use cache or something
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				client := newReflectionClient(ctx)
				serviceDesc, err := client.ResolveService(serviceName)
				if err != nil {
					log.Print(err.Error())
					return
				}
				methodDesc := serviceDesc.FindMethodByName(methodName)

				grpcBytes, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()
				if err != nil {
					log.Print(err.Error())
					return
				}
				protoBytes := grpcBytes[5:]
				inputType := methodDesc.GetInputType()

				// construct request message
				message := dynamic.NewMessage(inputType)
				if err := proto.Unmarshal(protoBytes, message); err != nil {
					log.Print(err.Error())
					return
				}

				jsonBytes, err := message.MarshalJSON()
				if err != nil {
					log.Print(err.Error())
					return
				}

				err = json.Unmarshal(fixtureJson, &fixtureMap)
				if err != nil {
					log.Print(err.Error())
					return
				}

				res, ok := fixtureMap[serviceName][methodName][string(jsonBytes)]
				if !ok {
					log.Print("no fixture found")
					return
				}

				// stubbed request message
				outputType := methodDesc.GetOutputType()
				resMessage := dynamic.NewMessage(outputType)
				if err := resMessage.UnmarshalJSON([]byte(res.Body)); err != nil {
					log.Print(err.Error())
					return
				}
				writeResponse(w, resMessage, res)
			}),
		})
	}
}
