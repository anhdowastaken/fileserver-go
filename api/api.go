package api

import (
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/anhdowastaken/fileserver-go/configurationmanager"
	"github.com/anhdowastaken/fileserver-go/logger"
	"github.com/anhdowastaken/fileserver-go/utilities"
)

type customResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *customResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *customResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, err := w.ResponseWriter.Write(b)

	return n, err
}

func authen(username string, password string) bool {
	cm := configurationmanager.New()

	httpConfig := cm.GetHTTPConfig()
	authenList := httpConfig.Authen

	if len(authenList) == 0 {
		return true
	}

	for _, v := range authenList {
		if v.Username == username {
			if v.Password == utilities.StringToMD5String(password) {
				return true
			}
		}
	}

	return false
}

// ValidateMiddleware is an HTTP midleware used to validate an authentication
func ValidateMiddleware(next http.Handler) http.Handler {
	cm := configurationmanager.New()
	httpConfig := cm.GetHTTPConfig()
	authenList := httpConfig.Authen

	// Bypass authentication if authen list is empty
	if len(authenList) == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		username, password, ok := r.BasicAuth()
		if ok {
			if !authen(username, password) {
				http.Error(w, "Unauthorized.", 401)
				return
			}
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "Unauthorized.", 401)
			return
		}
	})
}

// LoggingMiddleware is an HTTP middleware used to log all requests
func LoggingMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mlog := logger.New()
		id := uuid.New().String()
		mlog.Info.Printf("--> [%s] %s \"%s %s\"", id, r.RemoteAddr, r.Method, r.URL)
		w.Header().Set("X-Request-Id", id)

		cw := customResponseWriter{ResponseWriter: w}
		handler.ServeHTTP(&cw, r)

		statusCode := cw.status
		id = cw.Header().Get("X-Request-Id")
		mlog.Info.Printf("<-- [%s] %d %s", id, statusCode, http.StatusText(statusCode))
	})
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	cm := configurationmanager.New()
	httpConfig := cm.GetHTTPConfig()

	tmpl := template.Must(template.ParseFiles("template/index.html"))
	data := struct {
		MaxFileSize int
	}{
		MaxFileSize: httpConfig.MaxFileSize,
	}

	tmpl.Execute(w, data)
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	mlog := logger.New()

	var err error
	var localFilename string

	// ParseMultipartForm parses a request body as multipart/form-data
	cm := configurationmanager.New()
	httpConfig := cm.GetHTTPConfig()
	err = r.ParseMultipartForm(int64(httpConfig.MaxFileSize * 1024 * 1024))
	if err == nil {
		// Retrieve the file from form data
		var file multipart.File
		var fileHandler *multipart.FileHeader

		file, fileHandler, err = r.FormFile("file")
		if err == nil {
			defer file.Close()

			cm := configurationmanager.New()
			httpConfig := cm.GetHTTPConfig()
			fileServerDirectory := httpConfig.FileServerDirectory

			newFilename := r.FormValue("filename")
			if newFilename == "" {
				localFilename = utilities.SanitizeFilename(fileHandler.Filename)
			} else {
				localFilename = utilities.SanitizeFilename(newFilename)
			}
			localFilePath := filepath.Join(fileServerDirectory, localFilename)

			localFilenameTmp := fmt.Sprintf("%s.tmp", localFilename)
			localFilePathTmp := filepath.Join(fileServerDirectory, localFilenameTmp)

			mlog.Debug.Printf("Save %s", localFilePath)

			var f *os.File
			f, err = os.Create(localFilePathTmp)
			if err == nil {
				defer f.Close()

				_, err = io.Copy(f, file)
				if err == nil {
					err = os.Rename(localFilePathTmp, localFilePath)
				}
			}
		}
	}

	var tmpl *template.Template
	if err != nil {
		mlog.Critical.Printf("%+v", err)

		tmpl = template.Must(template.ParseFiles("template/error.html"))
		data := struct {
			Filename string
			Message  string
		}{
			Filename: localFilename,
			Message:  fmt.Sprintf("%+v", err),
		}

		tmpl.Execute(w, data)
	} else {
		tmpl = template.Must(template.ParseFiles("template/success.html"))
		data := struct {
			Filename string
		}{
			Filename: localFilename,
		}

		tmpl.Execute(w, data)
	}
}
