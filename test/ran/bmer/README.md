To test the feature, the test installs a consumer pod.
The container image for the pod is passed using an environment variable and it should align with the feature version:

for 4.11:
quay.io/redhat-cne/cloud-event-consumer:release-4.11
```bash
export CLOUD_EVENT_CONSUMER_IMAGE="registry.kni-qe-11.lab.eng.rdu2.redhat.com:5000/redhat-cne/cloud-event-consumer@sha256:c27cb8d1596193e0f7e1c09189de16b817f882211387c5d0a40447f74a1fc39c"
```

For 4.10:
quay.io/redhat-cne/cloud-event-consumer:release-4.10
```bash
export CLOUD_EVENT_CONSUMER_IMAGE="registry.kni-qe-11.lab.eng.rdu2.redhat.com:5000/redhat-cne/cloud-event-consumer@sha256:acf55f1358b7fa92b114d7d0af755e857d9eb983b8cd0d490035aef8e9c0d62a"
```
