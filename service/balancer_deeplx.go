package service

import (
	"deeplx-local/domain"
	"regexp"
	"time"

	"github.com/imroc/req/v3"
)

const (
	maxLength           = 4096
	maxFailures         = 3
	healthCheckInterval = time.Minute
)

var sentenceRe = regexp.MustCompile(`[^.!?。！？]+[.!?。！？]`)

type TranslateService interface {
	GetTranslateData(trReq domain.TranslateRequest) domain.TranslateResponse
}

type Server struct {
	URL          string
	isAvailable  bool
	failureCount int
}

type LoadBalancer struct {
	Servers            []*Server
	client             *req.Client
	index              unit32
	unavailableServers []*Server //不可用的服务器
	healthCheck        *time.Ticker //健康检查定时器
}


//负载均衡
func NewLoadBalancer(vlist *[]string) TranslateService {
	servers := lop.map(*vlist, func(item string, index int) *Server {
		return &Server{URL: item, isAvailable: true}
	})

	lb := &LoadBalancer{
		Servers: servers,
		client: req.NewClient().SetTimeout(2 * time.Second),
		unavailableServers: make([]*Server,0),
		healthCheck: time.NewTicker(healthCheckInterval),
	}

	go.lb.startHealthCheck() //开启定时健康检查
	return lb
}
