package cg

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Bananenpro/log"
)

type debugMessage struct {
	Severity DebugSeverity   `json:"severity"`
	Message  string          `json:"message"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type Logger struct {
	debugSocketsLock sync.RWMutex
	debugSockets     map[string]*debugSocket

	queue chan debugMessage

	printMessages bool

	closed bool
}

func NewLogger(printMessages bool) *Logger {
	l := &Logger{
		debugSockets:  make(map[string]*debugSocket),
		queue:         make(chan debugMessage, 32),
		printMessages: printMessages,
	}

	go func() {
		for {
			message, ok := <-l.queue
			if !ok {
				break
			}

			data, err := json.Marshal(message)
			if err != nil {
				log.Errorf("Failed to encode debug message: %s", err)
				continue
			}

			l.debugSocketsLock.RLock()
			for _, socket := range l.debugSockets {
				if active := socket.severities[message.Severity]; !active {
					continue
				}
				socket.send(data)
			}
			l.debugSocketsLock.RUnlock()
		}
	}()

	return l
}

func (l *Logger) Trace(format string, a ...any) {
	l.Log(DebugTrace, nil, format, a...)
}

func (l *Logger) Info(format string, a ...any) {
	l.Log(DebugInfo, nil, format, a...)
}

func (l *Logger) Warning(format string, a ...any) {
	l.Log(DebugWarning, nil, format, a...)
}

func (l *Logger) Error(format string, a ...any) {
	l.Log(DebugError, nil, format, a...)
}

func (l *Logger) TraceData(data any, format string, a ...any) {
	l.Log(DebugTrace, data, format, a...)
}

func (l *Logger) InfoData(data any, format string, a ...any) {
	l.Log(DebugInfo, data, format, a...)
}

func (l *Logger) WarningData(data any, format string, a ...any) {
	l.Log(DebugWarning, data, format, a...)
}

func (l *Logger) ErrorData(data any, format string, a ...any) {
	l.Log(DebugError, data, format, a...)
}

func (l *Logger) Log(severity DebugSeverity, data any, format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	var dataJson json.RawMessage
	if data != nil {
		if d, ok := data.([]byte); ok {
			dataJson = d
		} else {
			var err error
			dataJson, err = json.Marshal(data)
			if err != nil {
				log.Errorf("Failed to encode debug message data: %s", err)
				return
			}
		}
	}

	if l.printMessages {
		switch severity {
		case DebugTrace:
			log.Tracef("%s : %s", message, dataJson)
		case DebugInfo:
			log.Infof("%s : %s", message, dataJson)
		case DebugWarning:
			log.Warnf("%s : %s", message, dataJson)
		case DebugError:
			log.Errorf("%s : %s", message, dataJson)
		}
	}

	if !l.closed {
		l.queue <- debugMessage{
			Severity: severity,
			Message:  message,
			Data:     dataJson,
		}
	}
}

func (l *Logger) addDebugSocket(socket *debugSocket) {
	l.debugSocketsLock.Lock()
	l.debugSockets[socket.id] = socket
	l.debugSocketsLock.Unlock()
}

func (l *Logger) disconnectDebugSocket(id string) {
	l.debugSocketsLock.RLock()
	socket, ok := l.debugSockets[id]
	l.debugSocketsLock.RUnlock()
	if !ok {
		return
	}

	socket.disconnect()

	l.debugSocketsLock.Lock()
	delete(l.debugSockets, id)
	l.debugSocketsLock.Unlock()
}

func (l *Logger) Close() error {
	l.closed = true
	close(l.queue)
	return nil
}
