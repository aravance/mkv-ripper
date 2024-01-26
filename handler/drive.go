package handler

import (
	"log"

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
	var movie *gomdb.MovieResult
	if status == drive.StatusReady || status == drive.StatusMkv {
		disc := d.driveManager.GetDisc()
		info, found := d.discdb.GetDiscInfo(disc.Uuid)
		if found {
			main := util.GuessMainTitle(info)
			movie, _ = util.GetMovie(d.omdbapi, main.Name)
			if movie != nil {
				log.Println("got movie", *movie)
			} else {
				log.Println("could not get movie", info.Name)
			}
		}
	}
	var wf *model.Workflow
	if d.driveManager.HasDisc() {
		disc := d.driveManager.GetDisc()
		wf = d.workflowManager.GetWorkflow(disc.Uuid)
	}
	return render(c, driveview.Show(status, wf, movie))
}

func (d DriveHandler) GetDriveStatus(c echo.Context) error {
	status := d.driveManager.Status()
	var wf *model.Workflow
	if d.driveManager.HasDisc() {
		disc := d.driveManager.GetDisc()
		wf = d.workflowManager.GetWorkflow(disc.Uuid)
	}
	return render(c, driveview.Status(status, wf))
}
