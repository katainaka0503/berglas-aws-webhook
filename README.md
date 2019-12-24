# Berglas AWS Webhook
This is a sample project **for learning** mutating webhook implementation, and implemented referencing [Berglas](https://github.com/GoogleCloudPlatform/berglas)

Berglas AWS is a tool inspired by [GCP's Berglas](https://github.com/GoogleCloudPlatform/berglas) and command line tool and library for storing and retrieving secrets from AWS Secrets Manager.

This repository is a webhook part of Berglas AWS. The CLI implementation is [here](https://github.com/katainaka0503/berglas-aws).

## How to deploy
You need to deploy [cert-manager](http://github.com/jetstack/cert-manager) v0.12.0 before deploy this webhook.

```sh
$ kubectl apply -f ./deploy
```

## Usage
When you try to deploy Pod from YAML with env var in format of `berglas-aws://<ARN>` as below,
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
    - name: server
      image: server:any-tag
      command: ["server"]
      env:
        - name: API_KEY 
          value: berglas-aws://arn:aws:secretsmanager:<REGION>:<ACCOUNT_ID>:secret:<SECRET_ID>
```

this webhook mutates to YAML as below.
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: server
spec:
  initContainers:
    - name: copy-berglas-aws-bin
      image: katainaka0503/berglas-aws:latest
      imagePullPolicy: IfNotPresent
      command: ["sh", "-c", "cp $(which berglas-aws) /berglas-aws/bin/"]
      volumeMounts:
        - name: berglas-aws-bin
          mountPath: /berglas-aws/bin/
  containers:
    - name: server
      image: katainaka0503/server
      imagePullPolicy: Always
      command: ["/berglas-aws/bin/berglas-aws"]
      args: ["exec", "--", "server"]
      env:
        - name: API_KEY 
          value: berglas-aws://arn:aws:secretsmanager:<REGION>:<ACCOUNT_ID>:secret:<SECRET_ID>
      volumeMounts:
        - name: berglas-aws-bin
          mountPath: /berglas-aws/bin/
  volumes:
    - name: berglas-aws-bin
      emptyDir:
        medium: Memory
```
