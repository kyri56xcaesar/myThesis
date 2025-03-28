package main

import (
	"github.com/kyri56xcaesar/minioth"
)

func main() {
	m := minioth.NewMinioth("root", true, "minioth.db")
	srv := minioth.NewMSerivce(&m, "configs/minioth.env")
	srv.ServeHTTP()
}
