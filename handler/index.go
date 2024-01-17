package handler

import (
	"github.com/aravance/mkv-ripper/model"
	indexview "github.com/aravance/mkv-ripper/view/index"
	"github.com/labstack/echo/v4"
)

type IndexHandler struct {
}

func NewIndexHandler() IndexHandler {
	return IndexHandler{}
}

func (i IndexHandler) GetIndex(c echo.Context) error {
	workflows := model.LoadExistingWorkflows()
	return render(c, indexview.Show(workflows))
}
