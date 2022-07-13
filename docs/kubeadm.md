## Create a kubernetes cluster using Kubeadm

Docker comes with the native cgroup driver cgroupfs on Ubuntu 20.04 LTS. Modify the `10-kubeadm.conf` file in the
`/etc/systemd/system/kubelet.service.d` directory on all VMs.  The `kubeadm init ...` command may fail on
the master node if this is not corrected.

```
$ sudo vi /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
// Add " --cgroup-driver=cgroupfs" to the end of the environment variable
// Environment="KUBELET_KUBECONFIG_ARGS..."
```

Manually start the cluster on the master node.

```
$ sudo kubeadm init --pod-network-cidr=10.244.0.0/16
...                     // The output includes the "kubeadm join ..." command for worker nodes to join
kubeadm join 192.168.122.135:6443 –-token rbmp1g.pg798r0cshk4qvbw \
        –-discovery-token-ca-cert-hash sha256:5888010a3c67f2842279e5a22c496e3945e6ff6678f0ccae8d4cf03a350de4c01
```

## Run the `kubeadm join ...` command on worker nodes

Run the `kubeadm join ...` command on each node to join the cluster.

```
$ sudo kubeadm join 192.168.122.135:6443 –-token rbmp1g.pg798r0cshk4qvbw \
        –-discovery-token-ca-cert-hash sha256:5888010a3c67f2842279e5a22c496e3945e6ff6678f0ccae8d4cf03a350de4c01
...
This node has joined the cluster...
Run 'kubectl get nodes' on the control-plane to see this node join the cluster.
```

## Check the cluster on the master node.

Copy the configuration file to your `~/.kube` directory and run the `kubectl get nodes` command.

```
$ mkdir -p $HOME/.kube
$ sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
$ sudo chown $(id -u):$(id -g) $HOME/.kube/config
$ kubectl get nodes
```

The status of each node will be shown as `NotReady` at this point. It is because there is no CNI plugin
installed yet.
