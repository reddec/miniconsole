package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/reddec/miniconsole/assets"
)

type Config struct {
	Serve Serve `command:"serve" description:"serve HTTP UI"`
}

type Serve struct {
	MinioEndpoint string `long:"minio-endpoint" env:"MINIO_ENDPOINT" description:"Minio endpoint address" default:"localhost:9000"`
	MinioKeyID    string `long:"minio-key-id" env:"MINIO_KEY_ID" description:"Minio access key ID" default:"minioadmin"`
	MinoAccessKey string `long:"mino-access-key" env:"MINO_ACCESS_KEY" description:"Mino access secret" default:"minioadmin"`
	MinioSSL      bool   `long:"minio-ssl" env:"MINIO_SSL" description:"Use SSL to connect to Mino"`
	Bind          string `long:"bind" env:"BIND" description:"Binding address" default:"localhost:9001"`
	MaxObjects    int    `long:"max-objects" env:"MAX_OBJECTS" description:"Limit maximum number of objects to list in buckets" default:"1024"`
	Templates     string `long:"templates" env:"TEMPLATES" description:"Custom templates dir"`
}

func main() {
	var config Config
	_, err := flags.Parse(&config)
	if errors.Is(err, flags.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatal(err)
	}
}

func (serve *Serve) Execute([]string) error {
	mc, err := minio.New(serve.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(serve.MinioKeyID, serve.MinoAccessKey, ""),
		Secure: serve.MinioSSL,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	router := gin.Default()

	if serve.Templates != "" {
		router.LoadHTMLGlob(serve.Templates + "/**")
	} else {
		router.SetHTMLTemplate(assets.Templates())
	}

	srv := &server{
		maxList: serve.MaxObjects,
		mc:      mc,
	}
	router.StaticFS("/static", http.FS(assets.Static()))
	router.GET("/", srv.viewBuckets)
	router.POST("/buckets", srv.createBucket)
	router.POST("/buckets/trash", srv.removeBucket)

	router.GET("/object/raw", srv.objectData)
	router.GET("/object", srv.objectInfo)
	router.POST("/object/trash", srv.removeObject)

	router.GET("/objects", srv.listObjects)
	router.POST("/objects", srv.uploadFile)

	return router.Run(serve.Bind)
}

type server struct {
	maxList int
	mc      *minio.Client
}

func (srv *server) viewBuckets(gctx *gin.Context) {
	type responseType struct {
		Buckets []minio.BucketInfo
	}

	list, err := srv.mc.ListBuckets(gctx.Request.Context())
	if err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	gctx.HTML(http.StatusOK, "buckets.html", responseType{Buckets: list})
}

// GET /buckets/<bucket>/<prefix*>
func (srv *server) listObjects(gctx *gin.Context) {
	var request struct {
		Bucket string `form:"bucket"`
		Prefix string `form:"prefix"`
	}

	request.Prefix = "/"

	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	type responseType struct {
		Dirs       []dir
		Prefix     string
		BucketName string
		Prefixes   []minio.ObjectInfo
		Objects    []minio.ObjectInfo
	}

	ctx, cancel := context.WithCancel(gctx.Request.Context())
	defer cancel()

	iter := srv.mc.ListObjects(ctx, request.Bucket, minio.ListObjectsOptions{
		Prefix:  request.Prefix,
		MaxKeys: srv.maxList,
	})

	var files = make([]minio.ObjectInfo, 0)
	var prefixes = make([]minio.ObjectInfo, 0, srv.maxList/2)

	for item := range iter {
		if strings.HasSuffix(item.Key, "/") {
			prefixes = append(prefixes, item)
		} else {
			files = append(files, item)
		}
	}

	gctx.HTML(http.StatusOK, "bucket.html", responseType{
		BucketName: request.Bucket,
		Prefix:     request.Prefix,
		Objects:    files,
		Prefixes:   prefixes,
		Dirs:       dirs(request.Prefix),
	})
}

func (srv *server) objectInfo(gctx *gin.Context) {
	var request struct {
		Bucket   string `form:"bucket"`
		ObjectID string `form:"objectID"`
	}
	type response struct {
		Dirs       []dir
		Prefix     string
		BucketName string
		ObjectID   string
		Object     minio.ObjectInfo
	}
	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	info, err := srv.mc.StatObject(gctx.Request.Context(), request.Bucket, request.ObjectID, minio.StatObjectOptions{})
	if err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}
	gctx.HTML(http.StatusOK, "object.html", response{
		Prefix:     dirname(request.ObjectID),
		BucketName: request.Bucket,
		ObjectID:   request.ObjectID,
		Object:     info,
		Dirs:       dirs(request.ObjectID),
	})
}

func (srv *server) objectData(gctx *gin.Context) {
	var request struct {
		Bucket   string `form:"bucket"`
		ObjectID string `form:"objectID"`
	}

	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f, err := srv.mc.GetObject(gctx.Request.Context(), request.Bucket, request.ObjectID, minio.GetObjectOptions{})
	if err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	gctx.Header("Content-Type", stat.ContentType)
	gctx.Header("Content-Length", strconv.FormatInt(stat.Size, 10))
	gctx.Status(http.StatusOK)
	_, _ = io.Copy(gctx.Writer, f)
}

// POST /buckets
func (srv *server) createBucket(gctx *gin.Context) {
	var request struct {
		Bucket string `form:"bucket"`
	}

	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if err := srv.mc.MakeBucket(gctx.Request.Context(), request.Bucket, minio.MakeBucketOptions{}); err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	gctx.Redirect(http.StatusSeeOther, "objects?bucket="+url.PathEscape(request.Bucket))
}

// POST /buckets/<bucket>/trash
func (srv *server) removeBucket(gctx *gin.Context) {
	var request struct {
		Bucket string `form:"bucket"`
	}

	if err := gctx.BindUri(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if err := srv.mc.RemoveBucket(gctx.Request.Context(), request.Bucket); err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	gctx.Redirect(http.StatusSeeOther, "../..")
}

// POST /objects/trash/<bucket>/<objectID*>
func (srv *server) removeObject(gctx *gin.Context) {
	var request struct {
		Bucket   string `form:"bucket"`
		ObjectID string `form:"objectID"`
	}

	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if err := srv.mc.RemoveObject(gctx.Request.Context(), request.Bucket, request.ObjectID, minio.RemoveObjectOptions{}); err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	b := url.QueryEscape(request.Bucket)
	dir := dirname(request.ObjectID)
	gctx.Redirect(http.StatusSeeOther, "../objects?bucket="+b+"&prefix="+url.QueryEscape(dir))
}

func (srv *server) uploadFile(gctx *gin.Context) {
	var request struct {
		Prefix string                `form:"prefix"  binding:"required"`
		Bucket string                `form:"bucket" binding:"required"`
		File   *multipart.FileHeader `form:"content" binding:"required"`
	}

	if err := gctx.Bind(&request); err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	f, err := request.File.Open()
	if err != nil {
		gctx.AbortWithError(http.StatusBadRequest, err)
		return
	}
	defer f.Close()

	if !strings.HasSuffix(request.Prefix, "/") {
		request.Prefix += "/"
	}

	log.Printf("Request: %+v", request)

	_, err = srv.mc.PutObject(gctx.Request.Context(), request.Bucket, request.Prefix+request.File.Filename, f, request.File.Size, minio.PutObjectOptions{
		ContentType:        request.File.Header.Get("Content-Type"),
		ContentEncoding:    request.File.Header.Get("Content-Encoding"),
		ContentDisposition: request.File.Header.Get("Content-Disposition"),
		ContentLanguage:    request.File.Header.Get("Content-Language"),
	})
	if err != nil {
		gctx.AbortWithError(http.StatusBadGateway, err)
		return
	}

	gctx.Redirect(http.StatusSeeOther, "objects?bucket="+url.QueryEscape(request.Bucket)+"&prefix="+url.QueryEscape(request.Prefix))

}

func dirname(objectPath string) string {
	if objectPath == "" || objectPath == "/" {
		return "/"
	}
	p := strings.Split(objectPath, "/")
	if len(p) < 2 {
		return "/"
	}

	return "/" + strings.Join(p[:len(p)-1], "/")
}

type dir struct {
	Name   string
	Prefix string
}

func dirs(prefix string) []dir {
	if len(prefix) == 0 || prefix == "/" {
		return nil
	}
	parts := strings.Split(prefix, "/")
	var ans []dir
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		ans = append(ans, dir{
			Name:   p + "/",
			Prefix: strings.Join(parts[:i+1], "/"),
		})
	}

	return ans
}
