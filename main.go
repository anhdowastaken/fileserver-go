package main

import (
	// "fmt"
	"strings"
	// "path"
	// "net/url"
	// "sync"
	"flag"
	"io"
	"log/syslog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gorilla/mux"

	"github.com/anhdowastaken/fileserver-go/api"
	"github.com/anhdowastaken/fileserver-go/configurationmanager"
	"github.com/anhdowastaken/fileserver-go/logger"
	"github.com/anhdowastaken/fileserver-go/lumberjack"
)

const instanceName = "FILESERVER-GO"
const defaultConfigFile = "fileserver-go.conf"

func main() {
	mlog := logger.New()

	confPath := flag.String("c", "", "Config file of an instance")
	flag.Parse()

	// When start an instance, output log will be streamed to KERNEL LOG
	// Severity of log when streamed to syslog will be INFO
	logwriter, _ := syslog.New(syslog.LOG_INFO, "")

	mlog.SetStreamSingle(logwriter)

	mlog.SetPrefix(strings.ToUpper(instanceName))
	mlog.Info.Printf("Start %s", strings.ToUpper(instanceName))

	if *confPath == "" {
		mlog.Critical.Printf("Can not found config path in command line. Use default path instead: %s\n", defaultConfigFile)
		*confPath = defaultConfigFile
	}

	cm := configurationmanager.New()
	err := cm.Load(*confPath)
	if err != nil {
		mlog.Critical.Printf("Can not load config file %s: %+v\n", *confPath, err)
		os.Exit(1)
	}

	appConfig := cm.GetAppConfig()

	if appConfig.FilelogDestination != "" {
		err := os.MkdirAll(filepath.Dir(appConfig.FilelogDestination), 0755)
		if err != nil {
			mlog.Critical.Printf("Cannot make directories for logfile %s: %s", appConfig.FilelogDestination, err)
			os.Exit(1)
		}
	}

	// Configure streams for logger
	loggerStreams := make([]io.Writer, 0)
	lumberjackLog := &lumberjack.Logger{}
	if appConfig.FilelogDestination != "" {
		mlog.Info.Printf("Set log to %s", appConfig.FilelogDestination)
		lumberjackLog = &lumberjack.Logger{
			Filename:     appConfig.FilelogDestination,
			RotationTime: int(appConfig.LogRotationTime),
			MaxSize:      int(appConfig.MaxLogSize),
			LocalTime:    true,
		}
		loggerStreams = append(loggerStreams, lumberjackLog)
	}

	if len(loggerStreams) > 0 {
		mlog.SetStreamMulti(loggerStreams)
	}

	if appConfig.LogEnable == false {
		mlog.SetLevel(logger.DISABLE)
	}

	// Print config info
	mlog.Info.Printf("Log level: %s\n", logger.LOGLEVEL[appConfig.LogLevel])

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP)
	go func() {
		exitFlag := false

		for exitFlag != true {
			sig := <-sigs
			if sig == syscall.SIGINT || sig == syscall.SIGTERM || sig == syscall.SIGKILL {
				if sig == syscall.SIGINT {
					mlog.Info.Printf("Received SIGINT!")
				} else if sig == syscall.SIGTERM {
					mlog.Info.Printf("Received SIGTERM!")
				} else if sig == syscall.SIGKILL {
					mlog.Info.Printf("Received SIGKILL!")
				}

				exitFlag = true
				os.Exit(0)
			} else if sig == syscall.SIGHUP {
				mlog.Info.Printf("Received SIGHUP!")
				// Reload config
				err := cm.Load(*confPath)
				if err != nil {
					mlog.Critical.Printf("Can not reload config file %s: %+v\n", *confPath, err)
				} else {
					mlog.Info.Printf("Reload config file %s successfully\n", *confPath)
				}

				// Re-configure output log to KERNEL LOG
				// Severity of log when streamed to syslog will be INFO
				logwriter, _ := syslog.New(syslog.LOG_INFO, "")
				mlog.SetStreamSingle(logwriter)

				appConfig := cm.GetAppConfig()
				// Configure streams for logger
				loggerStreams := make([]io.Writer, 0)
				if appConfig.FilelogDestination != "" {
					_, err := os.OpenFile(appConfig.FilelogDestination, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
					if err != nil {
						mlog.Critical.Printf("Cannot access log file %s\n", appConfig.FilelogDestination)
					} else {
						mlog.Info.Printf("Set log to %s", appConfig.FilelogDestination)
						lumberjackLog.Close()
						lumberjackLog = &lumberjack.Logger{
							Filename:     appConfig.FilelogDestination,
							RotationTime: int(appConfig.LogRotationTime),
							MaxSize:      int(appConfig.MaxLogSize),
							LocalTime:    true,
						}
						loggerStreams = append(loggerStreams, lumberjackLog)
					}
				}

				if len(loggerStreams) > 0 {
					mlog.SetStreamMulti(loggerStreams)
				}

				if appConfig.LogEnable == false {
					mlog.SetLevel(logger.DISABLE)
				}

				// Print config info
				mlog.Info.Printf("Log level: %s\n", logger.LOGLEVEL[appConfig.LogLevel])
			}
		}
	}()

	// Create goroutine to serve HTTP REST API
	httpConfig := cm.GetHTTPConfig()

	router := mux.NewRouter()
	router.HandleFunc("/", api.IndexHandler).Methods("GET")
	router.HandleFunc("/upload", api.UploadHandler).Methods("POST")
	fileServer := api.NoDirListing(http.FileServer(http.Dir(httpConfig.FileServerDirectory)))
	router.PathPrefix("/download/").Handler(http.StripPrefix("/download/", fileServer)).Methods("GET")
	router.Use(api.ValidateMiddleware)

	address := httpConfig.Address
	srv := &http.Server{
		Handler:  api.LoggingMiddleware(router),
		Addr:     address,
		ErrorLog: mlog.Debug,
	}

	if httpConfig.SSL {
		mlog.Info.Printf("Start HTTPS server %s\n", address)
		mlog.Critical.Printf("%v+\n", srv.ListenAndServeTLS(httpConfig.CertFile, httpConfig.KeyFile))
	} else {
		mlog.Info.Printf("Start HTTP server %s\n", address)
		mlog.Critical.Printf("%v+\n", srv.ListenAndServe())
	}

	os.Exit(1)
}
