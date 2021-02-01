## GitLab Push EventListener

Creates an EventListener that listens for GitLab webhook events.

### Try it out locally:

1. To create the GitLab trigger and all related resources, run:

   ```bash
   kubectl apply -f examples/gitlab/
   ```

1. Port forward:

   ```bash
   kubectl port-forward service/el-gitlab 8080:8080
   ```

   **Note**: Instead of port forwarding, you can set the
   [`serviceType`](https://github.com/tektoncd/triggers/blob/master/docs/eventlisteners.md#serviceType)
   to `LoadBalancer` to expose the EventListener with a public IP.

1. Test by sending the sample payload.

   ```bash
   curl -v -H "@examples/gitlab/headers.txt" \
   --data-binary "@examples/gitlab/body.json" \
   http://localhost:8080
   ```

   The response status code should be `201 Created`

1. You should see a new TaskRun that got created:

   ```bash
   kubectl get taskruns | grep gitlab-run-
   ```
