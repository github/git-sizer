package meter

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Progress is an interface for a simple progress meter. Call
// `Start()` to begin reporting. `format` should include some kind of
// '%d' field, into which will be written the current count. The
// format should be constructed so that the new output covers up the
// old output (e.g., it should be fixed length or include some
// trailing spaces). A CR character will be added automatically.
//
// Call `Inc()` every time the quantity of interest increases. Call
// `Stop()` to stop reporting. After an instance's `Stop()` method has
// been called, it may be reused (starting at value 0) by calling
// `Start()` again.
type Progress interface {
	Start(format string)
	Inc()
	Add(delta int64)
	Done()
}

// progressMeter is a `Progress` that reports the current state every
// `period`.
type progressMeter struct {
	lock           sync.Mutex
	format         string
	period         time.Duration
	lastShownCount int64
	// When `ticker` is changed, that tells the old goroutine that
	// it's time to shut down.
	ticker *time.Ticker

	// `count` is updated atomically:
	count int64
}

func NewProgressMeter(period time.Duration) Progress {
	return &progressMeter{
		period: period,
	}
}

func (p *progressMeter) Start(format string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.format = "\r" + format
	atomic.StoreInt64(&p.count, 0)
	p.lastShownCount = -1
	ticker := time.NewTicker(p.period)
	p.ticker = ticker
	go func() {
		for {
			<-ticker.C
			p.lock.Lock()
			if p.ticker != ticker {
				// We're done.
				ticker.Stop()
				p.lock.Unlock()
				return
			}
			c := atomic.LoadInt64(&p.count)
			if c != p.lastShownCount {
				fmt.Fprintf(os.Stderr, p.format, c)
				p.lastShownCount = c
			}
			p.lock.Unlock()
		}
	}()
}

func (p *progressMeter) Inc() {
	atomic.AddInt64(&p.count, 1)
}

func (p *progressMeter) Add(delta int64) {
	atomic.AddInt64(&p.count, delta)
}

func (p *progressMeter) Done() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.ticker = nil
	c := atomic.LoadInt64(&p.count)
	fmt.Fprintf(os.Stderr, p.format+"\n", c)
}

// NoProgressMeter is a `Progress` that doesn't actually report
// anything.
type NoProgressMeter struct{}

func (p *NoProgressMeter) Start(string) {}
func (p *NoProgressMeter) Inc()         {}
func (p *NoProgressMeter) Add(int64)    {}
func (p *NoProgressMeter) Done()        {}
