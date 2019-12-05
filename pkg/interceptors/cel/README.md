# Experimental CEL interceptor

## Usage

```yaml
 
apiVersion: tekton.dev/v1alpha1
kind: EventListener
metadata:
  name: github-listener-interceptor
spec:
  serviceAccountName: tekton-triggers-example-sa
  triggers:
    - name: foo-trig
      interceptor:
        cel:
          expression: "headers.match('X-GitHub-Event', 'pull_request')"
          values:
            "pr.url": body.pull_request.url
            "pr.short_sha": truncate(body.head.sha, 7)
      bindings:
      - name: pipeline-binding
      template:
        name: pipeline-template
```

The `values` map, represents a set of key, value pairs, the value is an expression, which is evaluated, and then inserted into the returned payload at the key location.

e.g. in the example above, the body would end up with something like:

```json
{
  "pr": {
    "url": "https://api.github.com/repos/Codertocat/Hello-World/pulls/2",
    "short_sha": "ec26c3e"
}
```

In addition to the rest of the GitHub hook body that was received.
