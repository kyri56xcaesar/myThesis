package main

import (
	"github.com/kyri56xcaesar/minioth"
)

func main() {
	m := minioth.NewMinioth("root", false, "")
	srv := minioth.NewMSerivce(&m, "configs/minioth.env")
	srv.ServeHTTP()
}
