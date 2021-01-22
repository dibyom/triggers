source $(dirname $0)/e2e-common.sh

set -x
set -o errexit
set -o pipefail

#TODO:
# Namespace in kubectl commands
# Trap to kill PORT_FORWARD_PID if it exists

# ASSUMPTION(s):
# The EventListener/Trigger name is the same as the name of the foldername# The TriggerTemplate contains a single resourceTemplate named whose name starts with foldername-run-
# Each folder contains a headers.txt and a body.json file for testing
folder_name=gitlab
folder=${REPO_ROOT_DIR}/examples/${folder_name}
yaml_files=$(find ${folder}/ -name *.yaml | sort)
PORT_FORWARD_PID=""

trap "cleanup" EXIT SIGINT
cleanup() {
  kill ${PORT_FORWARD_PID} || true
  rm response.txt || true
  for file in ${yaml_files}; do
    kubectl delete -f ${file} || true
  done
}

# Apply YAML files
for file in ${yaml_files}; do
   kubectl apply -f ${file}
done

# Sleep to allow everything to be created
kubectl wait --for=condition=Available --timeout=10s eventlisteners/${folder_name}

# Port forward to EL
kubectl port-forward service/el-${folder_name} 8080:8080 &
PORT_FORWARD_PID=$! # Store PID of port-forward to kill later
sleep 2 # TODO: Is there a better way to do this?

# Make the curl call
set +e
status_code=$(curl -s -o "${folder}/response.txt" -w "%{http_code}" -H "@${folder}/headers.txt" --data-binary "@${folder}/body.json" http://localhost:8080)
# Print curl response always
echo "Response is: \n $(cat ${folder}/response.txt)"
if [ ${status_code} != "201" ]; then
    echo "Test failed. Not created."
    echo "Dumping EventListener Logs"
    kubectl logs -l eventlistener=${folder_name}
    exit 1
fi

