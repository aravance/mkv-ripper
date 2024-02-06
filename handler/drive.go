package handler

import (
	"log"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	driveview "github.com/aravance/mkv-ripper/view/drive"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

type DriveHandler struct {
	driveManager    drive.DriveManager
	workflowManager model.WorkflowManager
	discdb          drive.DiscDatabase
	omdbapi         *gomdb.OmdbApi
}

func NewDriveHandler(discdb drive.DiscDatabase, driveManager drive.DriveManager, workflowManager model.WorkflowManager, omdbapi *gomdb.OmdbApi) DriveHandler {
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
				movie, err = util.GetMovie(d.omdbapi, name)
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
