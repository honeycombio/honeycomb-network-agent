# Using the Honeycomb Network Agent in a Production Environment

Using the Honeycomb Network Agent in a Production environment is very similar to running it on a local development machine.

### Getting the Helm chart

If not already done, you can install the Honeycomb Helm chart repository where the agent's chart is maintained:

```sh
helm repo add honeycomb https://honeycombio.github.io/helm-charts
helm repo update
```

Next, install the Agent


### Configuration

Configuration happens through a Helm values.yaml file. The only thing you need to set is your Honeycomb API key with the remaining configuration options being optional.

Example values.yaml

```yaml
honeycomb:
    apikey: <honeycomb-apikey>
```

### Deploying the Agent



### Viewing data in Honeycomb

Once the agent is up and starting to monitor traffic, it publishes events for the HTTP traffic it can identify

