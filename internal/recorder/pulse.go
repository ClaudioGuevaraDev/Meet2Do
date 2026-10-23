package recorder

import (
	"errors"
	"fmt"

	"github.com/jfreymuth/pulse"
)

// ErrNoAudioServer indica que no se pudo conectar a PulseAudio/PipeWire.
var ErrNoAudioServer = errors.New("no se pudo conectar al servidor de audio (PulseAudio/PipeWire)")

// openPulse abre el micrófono por defecto y el monitor de la salida por defecto.
func openPulse() (mic, monitor Source, release func(), err error) {
	c, err := pulse.NewClient(pulse.ClientApplicationName("Meet2Do"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %w", ErrNoAudioServer, err)
	}
	src, err := c.DefaultSource()
	if err != nil {
		c.Close()
		return nil, nil, nil, fmt.Errorf("micrófono por defecto: %w", err)
	}
	sink, err := c.DefaultSink()
	if err != nil {
		c.Close()
		return nil, nil, nil, fmt.Errorf("salida de audio por defecto: %w", err)
	}
	mic = &pulseSource{c: c, name: "Meet2Do (micrófono)", device: pulse.RecordSource(src)}
	monitor = &pulseSource{c: c, name: "Meet2Do (sistema)", device: pulse.RecordMonitor(sink)}
	return mic, monitor, c.Close, nil
}

// pulseSource es una Source respaldada por un stream de grabación de PulseAudio.
type pulseSource struct {
	c      *pulse.Client
	name   string
	device pulse.RecordOption
	stream *pulse.RecordStream
}

func (p *pulseSource) Start(write func([]int16)) error {
	w := pulse.Int16Writer(func(s []int16) (int, error) {
		write(s)
		return len(s), nil
	})
	stream, err := p.c.NewRecord(w,
		p.device,
		pulse.RecordMono,
		pulse.RecordSampleRate(SampleRate),
		pulse.RecordLatency(0.1),
		pulse.RecordMediaName(p.name),
	)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", p.name, err)
	}
	p.stream = stream
	stream.Start()
	return nil
}

func (p *pulseSource) Close() error {
	if p.stream == nil {
		return nil
	}
	p.stream.Stop()
	p.stream.Close()
	return p.stream.Error()
}
