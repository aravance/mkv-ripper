package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/library"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "modernc.org/sqlite"
)

func main() {
	cfg := ParseConfigFile("mkv-ripper.toml")
	if cfg.Omdb == nil || cfg.Omdb.Apikey == "" {
		log.Fatalln("must set omdb apikey")
	}

	outdir := cfg.Rip
	targets := make([]ingest.Target, len(cfg.Targets))
	for i, t := range cfg.Targets {
		log.Println("target:", t)
		targets[i] = ingest.Target{
			URL: &url.URL{
				Scheme: t.Scheme,
				Host:   t.Host,
				Path:   t.Path,
			},
			Shafile:     t.Shafile,
			UseMovieDir: t.UseMovieDir,
		}
	}

	libs := make([]library.Library, 0, len(cfg.Libraries))
	for _, l := range cfg.Libraries {
		// don't log l itself, it contains the token
		if l.Url == "" || l.Token == "" {
			log.Println("library is missing url or token, skipping:", l.Type, l.Url)
			continue
		}
		switch strings.ToLower(l.Type) {
		case "jellyfin":
			log.Println("library: jellyfin", l.Url)
			libs = append(libs, library.NewJellyfin(l.Url, l.Token, l.User))
		default:
			log.Println("unsupported library type, skipping:", l.Type, l.Url)
		}
	}
	syncer := library.NewSyncer(libs...)
	if cfg.Tmdb != nil && cfg.Tmdb.Apikey != "" {
		log.Println("series lookup: tmdb")
		syncer.Lookup = library.NewTmdb(cfg.Tmdb.Apikey)
	}

	if logfile, err := os.OpenFile(path.Join(cfg.Log, "mkv.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	dbPath := path.Join(cfg.Data, "mkv-ripper.db")
	sqldb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalln("failed to open sqlite database", err)
	}
	defer sqldb.Close()

	var wfman workflow.WorkflowManager
	omdbapi := gomdb.Init(cfg.Omdb.Apikey)
	discdb, err := drive.NewSqliteDiscDatabase(sqldb)
	if err != nil {
		log.Fatalln("failed to initialize disc database", err)
	}

	migrateJsonDiscs(path.Join(cfg.Data, "discs.json"), discdb)

	handle := func(driveman drive.DriveManager) {
		handleDisc(discdb, wfman, driveman, omdbapi)
	}
	driveman := drive.NewUdevDriveManager(handle)
	wfman, err = workflow.NewSqliteWorkflowManager(sqldb, driveman, discdb, targets, outdir)
	if err != nil {
		log.Fatalln("failed to initialize workflow manager", err)
	}

	migrateJsonWorkflows(path.Join(cfg.Data, "workflows.json"), wfman)

	if syncer.Enabled() {
		wfman.OnIngested(func(wf *model.Workflow) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			q := library.QueryForWorkflow(wf)
			if err := syncer.AfterIngest(ctx, q, slices.Clone(wf.Collections)); err != nil {
				log.Println("error updating library collections for", q, err)
			}
		})
	}

	driveman.Start()
	defer driveman.Stop()

	for _, wf := range wfman.GetAllWorkflows() {
		if wf.Status == model.StatusRipping {
			wf.Status = model.StatusError
			wfman.Save(wf)
		}
		if wf.Status == model.StatusImporting {
			wf.Status = model.StatusPending
			wfman.Save(wf)
		}
		if wf.Status == model.StatusPending {
			if wf.Name != nil && wf.Year != nil {
				go func(w *model.Workflow) {
					wfman.Ingest(w)
				}(wf)
			}
		}
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman, omdbapi)
	driveHandler := handler.NewDriveHandler(discdb, driveman, wfman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, driveman, discdb, omdbapi)
	omdbHandler := handler.NewOmdbHandler(omdbapi)
	collectionsHandler := handler.NewCollectionsHandler(wfman, syncer)

	server.GET("/", indexHandler.GetIndex)
	server.GET("/history", indexHandler.GetHistory)
	server.GET("/movie/:imdbId", indexHandler.GetMovie)
	server.GET("/movie/:imdbId/collections", collectionsHandler.GetMovieCollections)
	server.POST("/movie/:imdbId/collections", collectionsHandler.PostMovieCollections)
	server.GET("/drive", driveHandler.GetDrive)
	server.GET("/drive/status", driveHandler.GetDriveStatus)
	server.GET("/disc/:discId/title/:titleId", workflowHandler.GetWorkflow)
	server.POST("/disc/:discId/title/:titleId", workflowHandler.PostWorkflow)
	server.GET("/disc/:discId/title/:titleId/edit", workflowHandler.EditWorkflow)
	server.GET("/disc/:discId/title/:titleId/status", workflowHandler.Status)
	server.GET("/disc/:discId/title/:titleId/collections", collectionsHandler.GetWorkflowCollections)
	server.POST("/disc/:discId/title/:titleId/collections", collectionsHandler.PostWorkflowCollections)
	// TODO make this a post
	server.GET("/disc/:discId/title/:titleId/rip", workflowHandler.RipTitle)
	server.GET("/omdb/search", omdbHandler.Search)

	go func() {
		if err := server.Start(fmt.Sprintf(":%d", cfg.Port)); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("server error", err)
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	<-sigchan

	log.Println("shutting down")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalln("server shutdown error", err)
	}
}

func handleDisc(
	discdb drive.DiscDatabase,
	wfman workflow.WorkflowManager,
	driveman drive.DriveManager,
	omdbapi *gomdb.OmdbApi,
) {
	disc := driveman.GetDisc()
	if disc == nil || (disc.Uuid == "" && disc.Label == "") {
		return
	}

	var info *makemkv.DiscInfo
	var found bool
	var err error
	info, found = discdb.GetDiscInfo(disc.Uuid)
	if !found || info.VolumeName == "" {
		info, err = driveman.GetDiscInfo()
		if err != nil {
			log.Println("error getting disc info:", disc, "err:", err)
			return
		}
		err = discdb.SaveDiscInfo(disc.Uuid, info)
		if err != nil {
			log.Println("error saving disc info:", disc, "err:", err)
			return
		}

		main := util.GuessMainTitle(info)
		if main == nil {
			log.Println("failed to guess main title")
			return
		}
		name := util.GuessName(info, main)
		wf, _ := wfman.NewWorkflow(disc.Uuid, main.Id, disc.Label, name)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if movie, err := util.GetMovie(name, omdbapi); err != nil {
				log.Println("failed to fetch movie details:", name)
			} else {
				wf.Name = &movie.Title
				wf.Year = &movie.Year
				wf.ImdbId = &movie.ImdbID
				wfman.Save(wf)
			}
		}()

		err = wfman.Start(wf)
		wg.Wait()
		if err != nil {
			log.Println("failed to rip title", err)
			return
		}
	}
}
