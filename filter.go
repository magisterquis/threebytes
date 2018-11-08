package main

/*
 * filter.go
 * Filter logs
 * By J. Stuart McMurray
 * Created 20181108
 * Last Modified 20181108
 */

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func filter(id string) {
	/* Tag which means output line */
	tag := fmt.Sprintf("[RX:%v]", id)

	/* Process each line */
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		/* Extract the important bits */
		parts := strings.SplitN(l, " ", 4)
		if 4 != len(parts) {
			continue
		}
		/* Make sure it's a line we want */
		if tag != parts[2] {
			continue
		}
		/* Unquote and print the string */
		s, err := strconv.Unquote(parts[3])
		if nil != err {
			log.Printf("Bad line %q", l)
			continue
		}
		fmt.Printf("%s", s)
	}
	if err := scanner.Err(); nil != err {
		log.Printf("Error: %v", err)
	}
}
