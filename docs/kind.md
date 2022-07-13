## Create a kubernetes cluster using Kind

Install kind following [these instructions](https://kind.sigs.k8s.io/docs/user/quick-start/).

Create a `kind-config.yaml` file:
```
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  # the default CNI will not be installed
  disableDefaultCNI: true
nodes:
- role: control-plane
- role: worker
- role: worker
```

To start the cluster, run:
```
kind create cluster --config kind-config.yaml
```

To delete the cluster, run:
```
kind delete cluster
```
