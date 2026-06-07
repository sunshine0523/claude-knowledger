package web

import (
	"embed"
	"io/fs"
)

//go:embed static/* templates/*
var assets embed.FS

func StaticFS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func TemplateFS() fs.FS {
	sub, err := fs.Sub(assets, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}
