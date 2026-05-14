package main

import "github.com/baditaflorin/go-common/server"

func main() {
	server.Run("go_substack_scraper", version, Handler)
}
