package main

import (
	"bufio"
	"fmt"
	"os"
)

func requestDetails(content map[string]interface{}) (details *MovieDetails, changed bool) {
	scanner := bufio.NewScanner(os.Stdin)
	changed = false
	var name string
	var year string
	var variant string
	fmt.Println("Found new device:", content["label"])
	if content["name"] != nil {
		name, _ = content["name"].(string)
	} else {
		changed = true
		fmt.Println("Name?")
		scanner.Scan()
		name = scanner.Text()
		content["name"] = name
	}
	if content["year"] != nil {
		year, _ = content["year"].(string)
	} else {
		changed = true
		fmt.Println("Year?")
		scanner.Scan()
		year = scanner.Text()
		content["year"] = year
	}
	if content["variant"] != nil {
		variant, _ = content["variant"].(string)
	} else {
		changed = true
		fmt.Println("Variant? [1] 4k  [2] 1080p  [3] 720p [4] 480p")
		scanner.Scan()
		switch scanner.Text() {
		case "1":
			variant = "4k"
		case "2":
			variant = "1080p"
		case "3":
			variant = "720p"
		case "4":
			variant = "480p"
		default:
			variant = "4k"
		}
		content["variant"] = variant
	}
	return &MovieDetails{name, year, variant}, changed
}
