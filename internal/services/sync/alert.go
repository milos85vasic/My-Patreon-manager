package sync

type Alert interface {
	Send(subject, body string)
}

type LogAlert struct{}

func (a *LogAlert) Send(subject, body string) {}
