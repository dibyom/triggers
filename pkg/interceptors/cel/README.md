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

