package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/aravance/mkv-ripper/model"
	"github.com/labstack/echo/v4"
)

type IndexHandler struct {
}

func NewIndexHandler() IndexHandler {
	return IndexHandler{}
}

func (i IndexHandler) GetIndex(c echo.Context) error {
	workflows := model.LoadExistingWorkflows()
	if len(workflows) == 0 {
		return c.String(http.StatusOK, fmt.Sprintf("No workflows in progress"))
	} else {
		workflowStrs := make([]string, len(workflows))
		for i, workflow := range workflows {
			workflowStrs[i] = fmt.Sprintf("%s: %s", workflow.Id, workflow.Label)
		}
		fmt.Println("making template")
		t := template.Must(template.New("index").Parse(`{{ range . }}<a href="/workflow/{{ .Id }}">{{ .Label }}</a>{{ end }}`))
		buf := &bytes.Buffer{}
		fmt.Println("executing template")
		t.Execute(buf, workflows)
		fmt.Println("done with template")
		return c.HTML(http.StatusOK, buf.String())
	}
}

