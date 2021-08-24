package assets

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed static/**
var static embed.FS

//go:embed views/**
var views embed.FS

func Static() fs.FS {
	return &withPrefix{
		prefix: "static/",
		data:   &static,
	}
}

func Templates() *template.Template {
	t, err := template.ParseFS(views, "**/*.html")
	if err != nil {
		panic(err)
	}

	return t
}

type withPrefix struct {
	prefix string
	data   fs.FS
}

func (tp *withPrefix) Open(name string) (fs.File, error) {
	name = tp.prefix + name
	return tp.data.Open(name)
}
