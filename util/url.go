package util

import (
	"fmt"
	"net/url"
)

func WorkflowUrl(discid string, titleid int, parts ...string) string {
	base := fmt.Sprintf("/disc/%s/title/%d", discid, titleid)
	u, _ := url.JoinPath(base, parts...)
	return u
}

func MovieUrl(imdbId string, parts ...string) string {
	u, _ := url.JoinPath("/movie/"+url.PathEscape(imdbId), parts...)
	return u
}
