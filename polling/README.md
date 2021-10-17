# Development notes

I setup the generated components by locally having the k8s.io/code-generator and k8s.io/apimachinery and running: (There is github versions too)

```
~/go/src/k8s.io/code-generator/generate-groups.sh all github.com/tektoncd/experimental/polling/pkg/client github.com/tektoncd/experimental/polling/pkg/apis "poll:v1alpha1"
```
This generates the client code dir and corresponding code and the zz_generatered.deepcopy.go code for each api


For the injection (knative)
```
hack/generate-knative.sh injection github.com/tektoncd/experimental/polling/pkg/client github.com/tektoncd/experimental/polling/pkg/apis "poll:v1alpha1"
```

Git Authentication

To allow for more than 60 api calls per hour, the polling requests must be authenticated, allowing a 5000 per hours limit.

Limitation on basic token auth for the time being:

```
   apiVersion: v1
   kind: Secret
   metadata:
     name: basic-user-token
     annotations:
       tekton.dev/git-0: https://api.github.com/repos/Jimbo4794/tekton-testing-repo # Described below
   type: kubernetes.io/basic-auth
   stringData:
     username: <username>
     token: <token>
```