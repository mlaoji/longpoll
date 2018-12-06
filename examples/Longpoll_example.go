package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/mlaoji/longpoll"
)

func main() {
	http.HandleFunc("/", home)
	http.HandleFunc("/wait", wait)

	go genRandomEvents()

	fmt.Println("Serving at http://127.0.0.1:9001")
	http.ListenAndServe("127.0.0.1:9001", nil)
}

func genRandomEvents() {
	events := map[string]string{
		"evt0": "test evt0",
		"evt1": "test evt1",
		"evt2": "test evt2",
		"evt3": "test evt3",
		"evt4": "test evt4",
		"evt5": "test evt5",
		"evt6": "test evt6",
	}

	longpoll.LongpollRedisConf = map[string]string{"host": "127.0.0.1:6379", "password": "test"}
	longpoll.LongpollChannel = "test-longpoll"
	client_id := "test"
	lp := longpoll.NewLongpoll()

	for {
		time.Sleep(time.Duration(rand.Intn(10000)) * time.Millisecond)

		evt := "evt" + fmt.Sprint(rand.Intn(len(events)))
		lp.Pub(client_id, evt, events[evt])
	}
}

func wait(w http.ResponseWriter, r *http.Request) {
	longpoll.LongpollRedisConf = map[string]string{"host": "127.0.0.1:6379", "password": "test"}
	longpoll.LongpollTimeout = 5
	longpoll.LongpollChannel = "test-longpoll"

	lp := longpoll.NewLongpoll()

	client_id := "test" //实际业务代码里应该使用UUID,超过LongpollClientTTL时刷新
	msg, err := lp.Run(client_id, w)

	fmt.Println(msg)
	fmt.Println(err)

	code := 0
	errmsg := "ok"

	switch err {
	case longpoll.ErrTimeout:
		code = 1
		errmsg = "timeout"
	}

	data := map[string]interface{}{
		"code":  code,
		"msg":   errmsg,
		"event": msg.Event,
		"time":  time.Now().Format("2006-01-02 15:04:05"),
		"data":  msg.Data,
	}

	res, _ := json.MarshalIndent(data, "", "")
	w.Write(res)
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `
<html>
<head>
    <title>longpoll example</title>
<script src="http://code.jquery.com/jquery-1.11.3.min.js"></script>
</head>
<body>
    <h1>longpoll example</h1>
	<h2>Here's whats happening:</h2>
    <ul id="rand-events"></ul>
<script>
    (function poll() {
        var pollUrl = "/wait";

        $.ajax({ url: pollUrl,
            success: function(res) {
                if (res) {
					if(res.code == 0) {
						$("#rand-events").append("<li>" + res.event+ " :  " + res.data +  " at " +res.time+ "</li>")
						poll();
						return;
					}
                
					if(res.code == 1) {
						console.log("No events, checking again.");

						poll();
						return;
					}
                }

                setTimeout(poll, 3000);
            }, dataType: "json",
        error: function (data) {
            console.log("Error in ajax request--trying again shortly...");
            setTimeout(poll, 3000);
        }
        });
    })();
</script>
</body>
</html>`)
}
