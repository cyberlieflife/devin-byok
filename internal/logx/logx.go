package logx

import (
	"fmt"
	"log"
	"os"
	"time"
)

var std = log.New(os.Stdout, "", 0)

func Infof(format string, args ...any) {
	std.Printf("%s INFO  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	std.Printf("%s WARN  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	std.Printf("%s ERROR %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
