package util

import "github.com/aravance/go-makemkv"

func GuessMainTitle(info *makemkv.DiscInfo) *makemkv.TitleInfo {
	if info == nil || len(info.Titles) == 0 {
		return nil
	}

	for _, t := range info.Titles {
		if t.SourceFileName == "00800.mpls" {
			return &t
		}
	}
	return &info.Titles[0]
}
