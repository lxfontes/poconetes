kubectl apply -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml


# patch it to have 2 ingresses "public" and "private"
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v0.48.1/deploy/static/provider/do/deploy.yaml


kubectl create ns argo
kubectl apply -n argo -f https://raw.githubusercontent.com/argoproj/argo-workflows/master/manifests/quick-start-postgres.yaml
