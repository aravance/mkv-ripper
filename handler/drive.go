package handler

import (
	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	driveview "github.com/aravance/mkv-ripper/view/drive"
	"github.com/labstack/echo/v4"
)

type DriveHandler struct {
	driveManager drive.DriveManager
}

func NewDriveHandler(driveManager drive.DriveManager) DriveHandler {
	return DriveHandler{driveManager}
}

func (d DriveHandler) GetDrive(c echo.Context) error {
	status := d.driveManager.Status()
	found := false
	var info makemkv.DiscInfo
	if status == drive.StatusReady || status == drive.StatusMkv {
		info, found = d.driveManager.GetDiscInfo()
	}
	return render(c, driveview.Show(status, info, found))
}
