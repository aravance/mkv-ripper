package main

import (
	"bufio"
	"fmt"
	"os"
)

type MovieDetails struct {
	name string
	year string
}

func requestDetails(content map[string]interface{}) (details *MovieDetails, changed bool) {
	scanner := bufio.NewScanner(os.Stdin)
	changed = false
	var name string
	var year string
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
	return &MovieDetails{name, year}, changed
}
