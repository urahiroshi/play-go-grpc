package main

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	proto "github.com/golang/protobuf/proto"
	tpb "github.com/golang/protobuf/proto/proto3_proto"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func proxyForGRPC(backendURL string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	dial := func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr)
	}
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLS:   dial,
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.Transport = transport
	return p, nil
}

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

func main() {
	p, err := proxyForGRPC("http://localhost:50052")
	if err != nil {
		log.Fatal(err.Error())
	}

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
				messageDesc := methodDesc.GetInputType()
				message := dynamic.NewMessage(messageDesc)

				grpcBytes, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()
				if err != nil {
					log.Print(err.Error())
					return
				}
				protoBytes := grpcBytes[5:]
				pb := new(tpb.Message)
				if err := proto.Unmarshal(protoBytes, pb); err != nil {
					log.Print(err.Error())
					return
				}

				if err := message.ConvertFrom(pb); err != nil {
					log.Print(err.Error())
					return
				}
				json, err := message.MarshalJSON()
				if err != nil {
					log.Print(err.Error())
					return
				}
				log.Print(json)
				log.Printf("")

				// msgDesc, err := desc.LoadMessageDescriptorForMessage(pb)
				// if err != nil {
				// 	log.Print(err.Error())
				// 	return
				// }
				// fieldDescs := msgDesc.GetFields()
				// log.Print(fieldDescs)

				// // buf := proto.NewBuffer(protoBytes)
				// // str1, err := buf.DecodeStringBytes()
				// // if err != nil {
				// // 	log.Print(err.Error())
				// // 	return
				// // }
				// // log.Print(str1)

				// str2 := pb.String()
				// log.Print(str2)

				// msg, err := dynamic.AsDynamicMessage(pb)
				// if err != nil {
				// 	log.Print(err.Error())
				// 	return
				// }

				// // for fieldDesc := range fieldDescs {
				// // 	msg.ForEachMapFieldEntry(fieldDesc, func(key, val interface{}) {
				// // 		log.Print("field:")
				// // 		log.Print(key)
				// // 		log.Print(val)
				// // 	})
				// // }

				// json, err := msg.MarshalJSON()
				// if err != nil {
				// 	log.Print(err.Error())
				// 	return
				// }
				// log.Print(json)
				// log.Printf("")

				p.ServeHTTP(w, r)
			}),
		})
	}
}
