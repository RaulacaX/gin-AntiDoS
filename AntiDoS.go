package AntiDoS

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Engine struct {
	max_requests int           // Maximum requests allowed
	ban_time     time.Duration // Ban duration
	refresh_time time.Duration // Time frame to refresh request count

	current_IPs     map[string]*IP_Requests
	blacklist       map[string]bool
	current_IPs_sem *sync.Mutex
	blacklist_sem   *sync.RWMutex
}

type IP_Requests struct {
	requests int
	o        *sync.Once
}

func DefaultDoSEngine() *Engine {
	return &Engine{
		max_requests: 30,
		ban_time:     5 * time.Minute,
		refresh_time: 4 * time.Second,

		current_IPs:     make(map[string]*IP_Requests),
		blacklist:       make(map[string]bool),
		current_IPs_sem: &sync.Mutex{},
		blacklist_sem:   &sync.RWMutex{},
	}
}

func createDoSEngine(max_requests int, ban_time, refresh_time time.Duration) *Engine {
	return &Engine{
		max_requests:    max_requests,
		ban_time:        ban_time,
		refresh_time:    refresh_time,
		current_IPs:     make(map[string]*IP_Requests),
		blacklist:       make(map[string]bool),
		current_IPs_sem: &sync.Mutex{},
		blacklist_sem:   &sync.RWMutex{},
	}
}

func (d *Engine) AntiDoSHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		client_ip := c.ClientIP()
		d.blacklist_sem.RLock()
		if _, exists := d.blacklist[client_ip]; exists {
			c.JSON(http.StatusUnauthorized, gin.H{"reason": "You Did too many requests"})
			d.blacklist_sem.RUnlock()
			return
		}
		d.blacklist_sem.RUnlock()
		d.current_IPs_sem.Lock()
		client_data, exists := d.current_IPs[client_ip]
		if !exists {
			d.current_IPs[client_ip] = &IP_Requests{
				requests: 1,
				o:        &sync.Once{},
			}
			go d.RequestsHandler(client_ip)
		} else {
			client_data.requests++
			if client_data.requests > d.max_requests {
				d.current_IPs_sem.Unlock()
				client_data.o.Do(func() { d.ban_IP(client_ip) })
				c.JSON(http.StatusUnauthorized, gin.H{"reason": "You Did too many requests"})
				return
			}
		}
		d.current_IPs_sem.Unlock()
	}
}

func (d *Engine) RequestsHandler(IP string) {
	d.current_IPs_sem.Lock()
	value, _ := d.current_IPs[IP]
	d.current_IPs_sem.Unlock()
	if value == nil {
		return
	}
	dont_changed_counter := 0
	for {
		time.Sleep(d.refresh_time)
		d.blacklist_sem.RLock()
		if _, exists := d.blacklist[IP]; exists {
			d.blacklist_sem.RUnlock()
			break
		}
		d.blacklist_sem.RUnlock()
		d.current_IPs_sem.Lock()
		if value.requests == 0 {
			dont_changed_counter = 0
		} else if value.requests <= d.max_requests {
			value.requests = 0
			dont_changed_counter++
		}
		if dont_changed_counter == 4 {
			delete(d.current_IPs, IP)
			d.current_IPs_sem.Unlock()
			break
		}
		d.current_IPs_sem.Unlock()
	}
}
func (d *Engine) ban_IP(IP string) {
	d.blacklist_sem.Lock()
	d.blacklist[IP] = true
	d.blacklist_sem.Unlock()
	d.current_IPs_sem.Lock()
	delete(d.current_IPs, IP)
	d.current_IPs_sem.Unlock()
	go d.unban_IP(IP)
}

func (d *Engine) unban_IP(IP string) {
	time.Sleep(d.ban_time)
	d.blacklist_sem.Lock()
	delete(d.blacklist, IP)
	d.blacklist_sem.Unlock()
}
