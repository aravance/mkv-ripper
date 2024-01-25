package handler

import (
	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	driveview "github.com/aravance/mkv-ripper/view/drive"
	"github.com/labstack/echo/v4"
)

type DriveHandler struct {
	driveManager drive.DriveManager
	discdb       drive.DiscDatabase
}

func NewDriveHandler(discdb drive.DiscDatabase, driveManager drive.DriveManager) DriveHandler {
	return DriveHandler{driveManager, discdb}
}

func (d DriveHandler) GetDrive(c echo.Context) error {
	status := d.driveManager.Status()
	found := false
	var info *makemkv.DiscInfo
	if status == drive.StatusReady || status == drive.StatusMkv {
		disc := d.driveManager.GetDisc()
		info, found = d.discdb.GetDiscInfo(disc.Uuid)
	}
	return render(c, driveview.Show(status, info, found))
}
