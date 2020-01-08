package main

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"golang.org/x/net/http2"
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
				p.ServeHTTP(w, r)
			}),
		})
	}
}
