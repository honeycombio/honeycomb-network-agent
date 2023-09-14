# Load Testing

1. Deploy echo server

    ```shell
    k apply -f echoserver.yaml
    ```

2. Build & deploy agent

    ```shell
    make docker-build
    HONEYCOMB_API_KEY=abc make apply-network-agent
    ```

3. Start load test

    ```shell
    locust
    ```

4. Run load test with 5-6k concurrent users at 100 users/s ramp up. It's not always going to get messed up, but when it does it will usually happen within a minute or two of starting the load test.

5. Watch the data in Honeycomb. You can tell when the matching starts to break down. You can see it in Honeycomb on a duration heatmap (it starts to first get much larger than average, and then goes negative).

6. Tear down echo server

    ```shell
    k delete -f echoserver.yaml
    ```
