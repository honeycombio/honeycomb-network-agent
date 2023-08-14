package assemblers

import "fmt"

func Error(t string, s string, a ...interface{}) {
	errorsMapMutex.Lock()
	errors++
	nb, _ := errorsMap[t]
	errorsMap[t] = nb + 1
	errorsMapMutex.Unlock()
	if logLevel >= 0 {
		fmt.Printf(s, a...)
	}
}
func Info(s string, a ...interface{}) {
	if logLevel >= 1 {
		fmt.Printf(s, a...)
	}
}
func Debug(s string, a ...interface{}) {
	if logLevel >= 2 {
		fmt.Printf(s, a...)
	}
}