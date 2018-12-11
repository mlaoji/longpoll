package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mlaoji/longpoll"
)

func main() {
	http.HandleFunc("/", home)
	http.HandleFunc("/getClientId", getClientId)
	http.HandleFunc("/doEvent", genRandomEvents)
	http.HandleFunc("/wait", wait)

	fmt.Println("Serving at http://127.0.0.1:9001")
	http.ListenAndServe("127.0.0.1:9001", nil)
}

func genRandomEvents(w http.ResponseWriter, r *http.Request) { // {{{
	client_id := r.URL.Query().Get("client_id")

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

	evt := "evt" + fmt.Sprint(rand.Intn(len(events)))

	lp := longpoll.NewLongpoll()
	lp.Pub(client_id, evt, events[evt])

	w.Write([]byte("do " + evt + " ok"))
} // }}}

func wait(w http.ResponseWriter, r *http.Request) { // {{{
	client_id := r.URL.Query().Get("client_id")

	longpoll.LongpollRedisConf = map[string]string{"host": "127.0.0.1:6379", "password": "test"}
	longpoll.LongpollTimeout = 5
	longpoll.LongpollChannel = "test-longpoll"

	lp := longpoll.NewLongpoll()

	msg, err := lp.Run(client_id, w)

	fmt.Println(msg)

	code := 0
	errmsg := "ok"

	switch err {
	case longpoll.ErrTimeout:
		code = 1
		errmsg = "timeout"
	case longpoll.ErrExpired:
		code = 2
		errmsg = "expired"
	}

	data := map[string]interface{}{
		"code":  code,
		"msg":   errmsg,
		"event": msg.Event,
		"time":  time.Now().Format("2006-01-02 15:04:05"),
		"data":  msg.Data,
	}

	res, _ := json.MarshalIndent(data, "", "")

	w.Write([]byte(strings.Replace(string(res), "\n", "", -1)))
} // }}}

func getClientId(w http.ResponseWriter, r *http.Request) {
	//实际业务代码里最好使用UUID(如https://github.com/google/uuid)
	client_id := "test" + strconv.Itoa(rand.Intn(9999))
	w.Write([]byte(client_id))
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `
<html>
<head>
    <title>longpoll example</title>
</head>
<body>
    <h1>longpoll example</h1>
	<h2>Here's whats happening:</h2>
    <ul id="rand-events"></ul>
<script>
  ajaxGet("/getClientId", 
  function(data) {
	  clientId = data

	  poll(clientId)
  })

  function poll(clientId) {
        var pollUrl = "/wait?client_id="+clientId;

        ajaxGet(pollUrl,
            function(data) {
				res = eval("(" + data + ")")
                if (res) {
					if(res.code == 0) {
						append("rand-events", res.event+ " :  " + res.data +  " at " +res.time)

						poll(clientId);
						return;
					}
                
					if(res.code == 1) {
						console.log("No events, checking again.");

						poll(clientId);
						return;
					}

					if(res.code == 2) {
						console.log("expired, pls reload page.");
						append("rand-events", "expired at " +res.time)
						append("rand-events", "<a href='javascript:window.location.reload()'>reload</a>")

						return;
					}
                }

                setTimeout("poll(" +clientId+")", 3000);
            })

	  setTimeout('ajaxGet("/doEvent?client_id=' + clientId +'", function(s){})', 3000)
    }

function append(objId,html){
	newdiv=document.createElement("li");
	newdiv.innerHTML = html

    document.getElementById(objId).appendChild(newdiv)
}

function ajaxGet(url, fn) {
    var xhr = new XMLHttpRequest();
    xhr.open('GET', url, true);
    xhr.onreadystatechange = function() {
      if (xhr.readyState == 4 && xhr.status == 200 || xhr.status == 304) {
        fn.call(this, xhr.responseText);
      }
    };
    xhr.send();
} 
</script>
</body>
</html>`)
}
