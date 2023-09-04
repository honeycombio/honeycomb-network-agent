# Load test instructions

1. Deploy echo server

    ```shell
    k apply -f echoserver.yaml
    ```

2. Build & deploy agent

    ```shell
    make docker-build
    HONEYCOMB_API_KEY=abc make apply-ebpf-agent
    ```

3. Start load test
   
   ```shell
   locust
   ```

4. Run load test with 5-6k concurrent users at 100 users/s ramp up. It's not always going to get messed up, but when it does it will usually happen within a minute or two of starting the load test.
5. Watch the data in Honeycomb. You can tell when the matching starts to break down. You can see it in Honeycomb on a duration heatmap (it starts to first get much larger than average, and then goes negative). You can also see it in Honeycomb if oyu group by `http.request.echo.body` and `http.response.body` -- they will match initially, and mis-matched ones will start to pop up.
