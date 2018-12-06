package longpoll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mediocregopher/radix.v2/pubsub"
	"github.com/mediocregopher/radix.v2/redis"
)

var (
	ErrTimeout        = errors.New("time out")
	ErrClientClosed   = errors.New("client request closed")
	ErrExpired        = errors.New("client expired")
	ErrTooManyClients = errors.New("too many clients")
)

var (
	LongpollChannel   = "longpoll"
	LongpollTimeout   = 25
	LongpollClientTTL = 60
	LongpollMaxClient = 1000
	LongpollRedisConf = map[string]string{
		"host":     "127.0.0.1:6379",
		"password": "",
	}
)

type client struct {
	clientId string
	messages chan Message
	addtime  int
}

type Message struct {
	ClientId string
	Event    string
	Data     interface{}
}

type Longpoll struct {
	sync.Mutex
	clients map[string]*client
	timeout int
}

var l *Longpoll
var once sync.Once

func NewLongpoll() *Longpoll {
	if nil == l {
		once.Do(func() {
			ctx, cancel := context.WithCancel(context.Background())

			l = &Longpoll{
				timeout: LongpollTimeout,
			}

			//哨兵，负责监控并处理消息
			go l.sentinel(ctx, cancel)

			//监控哨兵, 异常重启
			go func() {
				for {
					select {
					case <-ctx.Done():
						fmt.Println("restart sentinel")

						ctx, cancel := context.WithCancel(context.Background())
						l.sentinel(ctx, cancel)
					}

					time.Sleep(1e9)
				}
			}()
		})
	}

	return l
}

func (this *Longpoll) sentinel(ctx context.Context, cancel context.CancelFunc) { // {{{
	sub, err := this.getSubClient()
	if nil != err {
		fmt.Printf("getSubClient err: %#v, context canceled\n", err)
		cancel()
		return
	}

	sub.Subscribe(LongpollChannel)

	subChan := make(chan *pubsub.SubResp)

	go func() {
		for {
			fmt.Println("wait recevie\n")

			subChan <- sub.Receive()

			ping := sub.Ping()
			if nil != ping.Err {
				fmt.Printf("ping err: %#v, context canceled\n", ping.Err)
				cancel()
				return
			}
		}
	}()

	//删除过期连接
	go func() {
		for {
			fmt.Println("total clients:", len(this.clients))
			fmt.Println("remove expired clients:")
			for k, v := range this.clients {
				if v.addtime+LongpollClientTTL < int(time.Now().Unix()) {
					fmt.Println(k)
					this.delClient(k)
				}
			}

			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(300e9)
			}
		}
	}()

	for {
		fmt.Println("wait subChan")

		select {
		case msg := <-subChan:
			if msg.Type == pubsub.Message {
				var m = Message{}
				if err = json.Unmarshal([]byte(msg.Message), &m); nil == err {
					if cli, ok := this.clients[m.ClientId]; ok {
						cli.messages <- m
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
} // }}}

func (this *Longpoll) Run(client_id string, w http.ResponseWriter) (msg Message, err error) { // {{{
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	cli, err := this.addClient(client_id)
	if nil != err {
		return
	}

	closed := w.(http.CloseNotifier).CloseNotify()

	select {
	case msg = <-cli.messages:
		this.delClient(msg.ClientId)
		fmt.Printf("run client msg:%#v\n", msg)
		return
	case <-time.After(time.Duration(this.timeout) * time.Second):
		fmt.Println("timeout")
		err = ErrTimeout
	case <-closed:
		fmt.Println("closed")
		err = ErrClientClosed
	}

	return
} // }}}

func (this *Longpoll) addClient(client_id string) (*client, error) { // {{{
	this.Lock()
	defer this.Unlock()

	if nil == this.clients {
		this.clients = make(map[string]*client)
	}

	if cli, ok := this.clients[client_id]; !ok {
		if len(this.clients) >= LongpollMaxClient {
			return nil, ErrTooManyClients
		}

		cli = &client{client_id, make(chan Message, 1), int(time.Now().Unix())}
		this.clients[client_id] = cli
	} else if this.clients[client_id].addtime+LongpollClientTTL < int(time.Now().Unix()) {
		return nil, ErrExpired
	}

	return this.clients[client_id], nil
} // }}}

func (this *Longpoll) delClient(client_id string) { // {{{
	this.Lock()
	defer this.Unlock()

	if nil != this.clients {
		if _, ok := this.clients[client_id]; ok {
			delete(this.clients, client_id)
		}
	}
} // }}}

func (this *Longpoll) Pub(client_id, event string, data interface{}) error { // {{{
	pub, err := this.getPubClient()
	if nil != err {
		return err
	}

	msg := Message{client_id, event, data}
	msg_string, err := json.MarshalIndent(msg, "", "")
	if nil != err {
		return err
	}

	pubmsg := strings.Replace(string(msg_string), "\n", "", -1)
	fmt.Printf("pub message:%#v\n", pubmsg)
	pub.Cmd("PUBLISH", LongpollChannel, pubmsg)

	return nil
} // }}}

func (this *Longpoll) getPubClient() (*redis.Client, error) { // {{{
	return this.getRedis()
} // }}}

func (this *Longpoll) getSubClient() (*pubsub.SubClient, error) { // {{{
	sub, err := this.getRedis()

	if nil != err {
		return nil, err
	}

	return pubsub.NewSubClient(sub), nil
} // }}}

func (this *Longpoll) getRedis() (*redis.Client, error) { // {{{
	host := LongpollRedisConf["host"]
	pass := LongpollRedisConf["password"]

	r, err := redis.DialTimeout("tcp", host, time.Second*30)
	if nil != err {
		return nil, err
	}

	if "" != pass {
		if err = r.Cmd("AUTH", pass).Err; err != nil {
			r.Close()
			return nil, err
		}
	}

	return r, nil
} // }}}
