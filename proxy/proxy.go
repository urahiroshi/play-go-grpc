package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"io/ioutil"
	// "encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
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

func printAsJson(b []byte, messageDesc *desc.MessageDescriptor) {
	message := dynamic.NewMessage(messageDesc)
	if err := proto.Unmarshal(b, message); err != nil {
		log.Print(err.Error())
		return
	}
	jsonBytes, err := message.MarshalJSON()
	if err != nil {
		log.Print(err.Error())
		return
	}
	log.Print(string(jsonBytes))
}

type responseWriteProxy struct {
	writer     http.ResponseWriter
	methodDesc *desc.MethodDescriptor
	http.ResponseWriter
}

func newResponseWriteProxy(
	writer http.ResponseWriter,
	methodDesc *desc.MethodDescriptor,
) *responseWriteProxy {
	wp := new(responseWriteProxy)
	wp.writer = writer
	wp.methodDesc = methodDesc
	return wp
}

func (wp *responseWriteProxy) Header() http.Header {
	return wp.writer.Header()
}

func (wp *responseWriteProxy) Write(b []byte) (int, error) {
	if len(b) > 5 {
		protoBytes := b[5:]
		messageDesc := wp.methodDesc.GetOutputType()
		printAsJson(protoBytes, messageDesc)
	}
	return wp.writer.Write(b)
}

func (wp *responseWriteProxy) WriteHeader(statusCode int) {
	log.Printf("statusCode=%d", statusCode)
	wp.writer.WriteHeader(statusCode)
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

				grpcBytes, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()
				if err != nil {
					log.Print(err.Error())
					return
				}
				protoBytes := grpcBytes[5:]
				messageDesc := methodDesc.GetInputType()
				printAsJson(protoBytes, messageDesc)

				// recover body
				r.Body = ioutil.NopCloser(bytes.NewReader(grpcBytes))

				wp := newResponseWriteProxy(w, methodDesc)

				p.ServeHTTP(wp, r)
				for key, values := range wp.Header() {
					log.Printf("key=%s, values=%s", key, values)
				}
			}),
		})
	}
}
