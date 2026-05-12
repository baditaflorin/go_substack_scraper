package main

import (
	"github.com/baditaflorin/go-common/config"
	"github.com/baditaflorin/go-common/server"
)

func main() {
	cfg := config.Load("go_substack_scraper", version)
	srv := server.New(cfg)
	srv.Mux.HandleFunc("/t/", Handler)
	srv.Mux.HandleFunc("/go_substack_scraper", Handler)
	srv.Start()
}
