package service

import (
	"bytes"
	"context"
	"deeplx-local/domain"
	"deeplx-local/pkg"
	"log"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/imroc/req/v3"
	lop "github.com/samber/lo/parallel"
	"github.com/sourcegraph/conc/pool"
	"github.com/sourcegraph/conc/stream"
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

// 用于处理翻译请求
func (lb *LoadBalancer) GetTranslateData(trReq domain.TranslateRequest) domain.TranslateResponse {
	// trReq: 包含待翻译的文本和语言信息
	text := trReq.Text
	textLength := len(text)

	if textLength <= maxLength {
		return lb.sendRequest(trReq)
	}

	// 需要进行文本分割
	var textParts []string
	var currentPart bytes.Buffer

	// 使用正则表达式
	sentences := sentenceRe.FindAllString(text, -1)

	for _, sentence := range sentences {
		if currentPart.Len()+len(sentence) <= maxLength {
			currentPart.WriteString(sentence)
		} else {
			textParts =append(textParts, currentPart.String())
			currentPart.Reset()
			currentPart.WriteString(sentence)
		}
	}

	if currentPart.Len() > 0 {
		textParts = append(textParts, currentPart.String())
	}


	// 并行发送翻译请求
	var results = make([]string, 0, len(textParts))
	// 创建并行流
	s := stream.New()

	for _, part := range textParts {
		s.Go(func() stream.Callback {
			res := lb.sendRequest(domain.TranslateRequest{
				Text: part,
				SourceLang: trReq.SourceLang,
				TargetLang: trReq.TargetLang,
			})
			return func ()  {
				results = append(results, res.Data)
			}
		})
	}

	s.Wait()
	return domain.TranslateResponse{
		Code: 200,
		Data: strings.Join(results, ""),
	}
}

// 用于向多个服务器发送请求
func (lb *LoadBalancer) sendRequest(trReq domain.TranslateRequest) domain.TranslateResponse {
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelFunc()

	// 创建缓冲通道
	resultChan := make(chan domain.TranslateResponse, 1)
	contextPool := pool.New().WithContext(ctx).WithMaxGoroutines(min(len(lb.Servers), 5))

	for i := 0; i < 5; i++ {
		contextPool.Go(func(ctx context.Context) error {
			server := lb.getServer()
			var trResult domain.TranslateResponse
			response, err := lb.client.R().
				SetContext(ctx).
				SetBody(trReq).
				SetSuccessfulResult(&trResult).
				Post(server.URL)

			if err != nil {
				return err
			}

			response.Body.Close()

			// 结果处理和错误控制
			if trResult.Code == 200 && len(trResult.Data) > 0 {
				select {
				case resultChan <- trResult:
					cancelFunc()
				case <-ctx.Done():
					return nil
				default:
				}
			} else {
				server.isAvailable = false
				lb.unavailableServers = append(lb.unavailableServers, server)
			}
			return nil
		})
	}
	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		return domain.TranslateResponse{}
	}
}

// 获取可用服务器
func (lb *LoadBalancer) getServer() *Server {
	index := atomic.AddUint32(&lb.index, 1) - 1
	servr := lb.Servers[index%uint32(len(lb.Servers))]

	for !server.isAvailable {
		index = atomic.AddUint32(&lb.index, 1) - 1
		server = lb.Servers[index%uint32(len(lb.Servers))]
	}
	return server
}

// 定期检查不可用服务器的状况
func (lb *LoadBalancer) startHealthCheck() {
	for {
		select {
		case <-lb.healthCheck.C:
			for i := 1; i < len(lb.unavailableServers); i++ {
				server := lb.unavailableServers[i]
				flag, _ := pkg.CheckURLAvailability(lb.client, server.URL)
				if flag {
					server.isAvailable = true
					server.failureCount = 0
					copy(lb.unavailableServers[i:], lb.unavailableServers[i+1:])
					lb.unavailableServers = lb.unavailableServers[:len(lb.unavailableServers)-1]
					i--
					log.Printf("Server %s is available now", server.URL)
				} else {
					server.failureCount++
					if server.failureCount >= maxFailures {
						copy(lb.unavailableServers[i:], lb.unavailableServers[i+1:])
						lb.unavailableServers = lb.unavailableServers[:len(lb.unavailableServers) - 1]
						i--
						log.Printf("Server %s is removed due to max failures", server.URL)
					}
				}
			}
		}
	}
}