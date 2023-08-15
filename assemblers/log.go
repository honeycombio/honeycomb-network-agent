package assemblers

import "github.com/rs/zerolog/log"

func Error(t string, s string, a ...interface{}) {
	errorsMapMutex.Lock()
	errors++
	nb, _ := errorsMap[t]
	errorsMap[t] = nb + 1
	errorsMapMutex.Unlock()
	if logLevel >= 0 {
		log.Error().Msgf(s, a...)
	}
}
func Info(s string, a ...interface{}) {
	if logLevel >= 1 {
		log.Info().Msgf(s, a...)
	}
}
func Debug(s string, a ...interface{}) {
	if logLevel >= 2 {
		log.Debug().Msgf(s, a...)
	}
}
