package service

type Service interface {
	Install() error
	Remove() error
	Run(onStart, onStop func() error) error
	
	LogError(text string) error
	LogWarning(text string) error
	LogInfo(text string) error
}
