package main

import (
	"crypto/tls"
	"golang.org/x/net/http2"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func proxyForGRPC(backendURL string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	dial := func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr)
	}
	transport := &http2.Transport{
		DialTLS: dial,
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.Transport = transport
	return p, nil
}

func main() {
	// director := func(request *http.Request) {
	// 	request.URL.Scheme = "https"
	// 	request.URL.Host = ":50052"
	// }
	// rp := &httputil.ReverseProxy{Director: director}
	p, err := proxyForGRPC("localhost:50052")
	if err != nil {
		log.Fatal(err.Error())
	}
	server := http.Server{
		Addr:    ":50051",
		Handler: p,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err.Error())
	}
}
