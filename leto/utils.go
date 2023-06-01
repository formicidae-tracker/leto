package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func FilenameWithSuffix(fpath string, iter int) string {
	res := strings.TrimSuffix(fpath, filepath.Ext(fpath))
	_, err := strconv.Atoi(strings.TrimPrefix(filepath.Ext(res), "."))
	if err == nil {
		res = strings.TrimSuffix(res, filepath.Ext(res))
	}
	return fmt.Sprintf("%s.%04d%s", res, iter, filepath.Ext(fpath))
}

func FilenameWithoutOverwrite(fpath string) (string, int, error) {
	iter := 0
	for {
		toTest := FilenameWithSuffix(fpath, iter)
		if _, err := os.Stat(toTest); err != nil {
			if os.IsNotExist(err) == true {
				return toTest, iter, nil
			}
			return "", -1, err
		}
		iter += 1
	}
}

type ByteSize int64

var prefixes = []string{"", "ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"}

func (s ByteSize) String() string {
	v := float64(s)
	prefix := ""
	for _, prefix = range prefixes {
		if math.Abs(v) <= 1024.0 {
			break
		}
		v /= 1024.0
	}
	return fmt.Sprintf("%.1f %sB", v, prefix)
}

type HumanDuration time.Duration

func (d HumanDuration) String() string {
	dd := time.Duration(d)
	if dd > 24*time.Hour {
		dd = dd.Round(time.Hour)
		days := dd.Truncate(24 * time.Hour)
		hours := int((dd - days).Hours())
		return fmt.Sprintf("%dd%dh", int64(days.Hours()/24.0), hours)
	}

	if dd > time.Hour {
		dd = dd.Round(time.Minute)
		hours := dd.Truncate(time.Hour)
		minutes := int((dd - hours).Minutes())
		return fmt.Sprintf("%dh%dm", int(hours.Hours()), minutes)
	}

	return dd.String()
}
