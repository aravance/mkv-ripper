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

func requestDetails(workflow *Workflow) MovieDetails {
	fmt.Println("Requesting details for disk:", workflow.Label)
	var name string
	var year string
	scanner := bufio.NewScanner(os.Stdin)

	if workflow.Name != nil {
		name = *workflow.Name
	} else {
		fmt.Println("Name?")
		scanner.Scan()
		name = scanner.Text()
	}

	if workflow.Year != nil {
		year = *workflow.Year
	} else {
		fmt.Println("Year?")
		scanner.Scan()
		year = scanner.Text()
	}

	return MovieDetails{name, year}
}
