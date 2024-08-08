docker build -t custom-scheduler .

docker tag custom-scheduler:latest rawatrahul24/custom-scheduler:latest

docker push rawatrahul24/custom-scheduler:latest

sleep 10

docker images -f "reference=rawatrahul24/custom-scheduler" -f "dangling=true" -q | xargs docker rmi

kubectl delete -f deploy/custom-sch-deployment.yaml

sleep 10


kubectl apply -f deploy/custom-sch-deployment.yaml

