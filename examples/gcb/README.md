1. Apply RBAC: `kubectl apply -f rbac.yaml`
2. Apply Trigger: `kubectl apply -f trigger.yaml`
3. Port forward to EventListener: `kubectl port-forward service/el-gh-listener 8080:8080`
4. Test Push Trigger:

```
 curl -v \
 -H 'X-GitHub-Event: push' \
 -H 'X-Hub-Signature: sha1=4b729ceb2af112473e4f660525868dd107daccf4' \
 -H 'Content-Type: application/json' \
 --data-binary '@push_payload.json' \
 http://localhost:8080

```

5. Test Pull Request Trigger:

```
 curl -v \
 -H 'X-GitHub-Event: pull_request' \
 -H 'X-Hub-Signature: sha1=a51fdff0227931196cae087f491a0d0bc75ad6b7' \
 -H 'Content-Type: application/json' \
 --data-binary '@pr_payload.json' \
 http://localhost:8080
```
