import time, random
from locust import HttpUser, task, between


class QuickstartUser(HttpUser):
    host = "http://localhost:80"
    wait_time = between(1, 2)

    @task
    def hello_greeting(self):
        i = random.randrange(1, 10)
        self.client.get("/?echo_body=" + str(i))