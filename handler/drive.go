package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/util"
	driveview "github.com/aravance/mkv-ripper/view/drive"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

type DriveHandler struct {
	driveManager    drive.DriveManager
	workflowManager workflow.WorkflowManager
	discdb          drive.DiscDatabase
	omdbapi         *gomdb.OmdbApi
}

func NewDriveHandler(discdb drive.DiscDatabase, driveManager drive.DriveManager, workflowManager workflow.WorkflowManager, omdbapi *gomdb.OmdbApi) DriveHandler {
	return DriveHandler{driveManager, workflowManager, discdb, omdbapi}
}

func (d DriveHandler) GetDrive(c echo.Context) error {
	status := d.driveManager.Status()
	disc := d.driveManager.GetDisc()
	var movie *gomdb.MovieResult
	var info *makemkv.DiscInfo
	if status == drive.StatusReady || status == drive.StatusMkv {
		var found bool
		info, found = d.discdb.GetDiscInfo(disc.Uuid)
		if found {
			main := util.GuessMainTitle(info)
			if main != nil {
				var err error
				name := util.GuessName(info, main)
				movie, err = util.GetMovie(name, d.omdbapi)
				if err != nil {
					log.Println("error fetching movie:", name, "err:", err)
					movie = nil
				}
			} else {
				log.Println("failed to guess title and name")
			}
			if movie != nil {
				log.Println("got movie", *movie)
			} else {
				log.Println("could not get movie", info.Name)
			}
		}
	}
	return render(c, driveview.Show(status, disc, movie, info))
}

func (d DriveHandler) GetDriveStatus(c echo.Context) error {
	status := d.driveManager.Status()
	var disc *drive.Disc
	if d.driveManager.HasDisc() {
		disc = d.driveManager.GetDisc()
	}
	return render(c, driveview.Status(status, disc))
}

func (d DriveHandler) RipTitle(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	status := d.driveManager.Status()
	if err != nil {
		return c.String(http.StatusNotFound, "no title found")
	}
	if status == drive.StatusEmpty {
		return c.String(http.StatusNotFound, "drive is empty")
	}
	if status != drive.StatusReady {
		return c.String(http.StatusNotFound, "drive is busy")
	}
	disc := d.driveManager.GetDisc()
	if disc.Uuid != discId {
		return c.String(http.StatusNotFound, "disc changed")
	}
	discInfo, ok := d.discdb.GetDiscInfo(discId)
	if !ok {
		return c.String(http.StatusNotFound, "disc info not found")
	}
	titleInfo := discInfo.Titles[titleId]
	name := util.GuessName(discInfo, &titleInfo)

	wf, ok := d.workflowManager.NewWorkflow(discId, titleId, disc.Label, name)

	if wf.Name == nil || *wf.Name == "" || wf.Year == nil || *wf.Year == "" {
		if movie, err := util.GetMovie(wf.OriginalName, d.omdbapi); err != nil {
			log.Println("failed to GetMovie", err)
		} else {
			wf.Name = &movie.Title
			wf.Year = &movie.Year
			wf.ImdbId = &movie.ImdbID
			d.workflowManager.Save(wf)
		}
	}

	go d.workflowManager.Start(wf)
	return c.Redirect(http.StatusSeeOther, util.WorkflowUrl(wf.DiscId, wf.TitleId))
}
