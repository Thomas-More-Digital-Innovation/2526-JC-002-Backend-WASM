
Writing GO for WASM/WASI
WASM is well known for delivering near native performance on the clients browser. With the new [WASI](https://wasi.dev/) support however, it is possible to run wasm on the backend solely, before this interface existed all I/O operations had to be done with JS and passed into the WASM module, which kind of defeats the purpose of wasm on the backend.

When I started my journey on WASM for the backend I knew of WASM only, so I went the classic route of having a GO app and writing to stdout/stderr and handling all I/O (DB requests, File handling) from JS. I then went on to deploy this on a cloudflare worker, as there is support for WASM in this way there. This is the old way of doing it called WAGI, which is based on the old principle of CGI, where an http request triggers a binary which communicates through stdout/stderr, in this case however the binary is in WASM.

Something about this way of working felt icky though, so I went on researching how to run WASM solely, then I stumbled on WASI (wasip1 & wasip2). This looked interesting, so I looked on how to compile to these proposals, go natively can compile to wasip1:
``` bash
GOOS=wasip1 GOARCH=wasm go build -o main.wasm main.go
```
But wasip1 does not (natively) support the creation of network sockets (there are libraries to achieve this), so it still felt wrong to stop there, so I went on to look at a more complete solution for running web server in WASM.

Enter wasip2, native support for opening network sockets. BUT: GO doesn't (as of the time of writing) support compiling to wasip2. [TinyGo](https://tinygo.org/), however does:
```bash
GOOS=wasip2 GOARCH=wasm tinygo build -o main.wasm main.go
```
To enable the serving of http requests I used ydnar's wasi-http-go/wasihttp package. This acts as the bridge between standard go libs and the wasi-http proxy component.
Next I looked for a way to run my shiny wasm binary. There are quite a few options, but for the sake of simplicity I went with [wasmtime](https://docs.wasmtime.dev/), as this was the one ydnar used as well. One catch I had to figure out is that you have to use [```wasmtime serve -Scli```](https://docs.wasmtime.dev/cli-options.html#serve) to serve this, as it's a wasi-http proxy component -Scli is used to enable WASI cli support, used for making I/O, like filesystem, clock, random and most importantly, sockets! So now we have an http server runnning with a WASN/WASI backend, but this is all still very much a development environment, how to make to production ready?

When I looked on how to deploy this, I first did some market research, then I stumbled on
[Fermyon](https://www.fermyon.com/), they advertise serverless WASM in kubernetes through Spin/SpinKube, now this was interesting! Sadly I did not get access to the Fermyon platform to test a deployment, so I cannot speak on how well their service works, but I did make a spin app and deployed it to a local k3s cluster, where I saw remarkably low resource usage when idling, 5m cpu and just 6Mi of RAM, I ran this on very old hardware (Xeon E5-2660 with HDD Yikes!), so my results for performance are not representative for today's hardware. I set up autoscaling with a HPA, which worked very well:
![[LoadTestHPA.png]]
Above is a load test performed with K6 in grafana cloud.

Despite the extremely low resource usage at idle, I still wondered about the claims from Fermyon of them running serverless and wondered what the results would be with my DIY cluster. So I installed KEDA and set up scaling based on incoming HTTP requests, with scale-to-zero for the wasm app itself. What I noticed is the readiness and liveliness checks are not actually needed, because of how fast the wasm runtime is able to spin up the extra servers. I was able to get the cold starts down to ~3s with a sata ssd, which seems slow, until you remember that the hardware is 14 years old. Most of the time is waiting for the k8s resources to be created. Below is a load test of this setup with KEDA:
![[LoadTestKEDA.png]]

In conclusion, the tradeoff of going serverless on your own k8s cluster doesn't make sense unless you have an immense amount of applications that won't have a big load. WASM doesn’t eliminate cold starts in kubernetes, it just makes the container startup negligible compared to orchestration overhead.

But maybe clouflare can offer a solution with their new workers beta for containers, I deployed the template which was quite easy thanks to the excellent documentation/wrangler automation. 
![[Pasted image 20260423201318.png]]

Then all that's left is to switch out the default linux/amd64 container for a wasm equivalent, so I read the documentation from cloudflare, when I stumbled upon this: ["Your container image must be able to run on the `linux/amd64` architecture, but aside from that, has few limitations."](https://developers.cloudflare.com/containers/get-started/#the-container-image). this means what I was trying to achieve is impossible/unsupported. Maybe in the future cloudflare will support it, as this technology is quite similar to their standard workers platform with less overhead, since the isolation is built in, as opposed to having to build it around the V8 engine. 

## Reproducing Steps

Here is how you can reproduce the experiments located in this repository:

### 1. Standard Go API
The code for the standard Go API (using Gin and Bun with PostgreSQL) is located in `code/api`.

**Local Testing:**
1. Navigate to the api directory:
   ```bash
   cd code/api
   ```
2. Ensure you have a `.env` file with your `DB_DSN` configured, for example:
   ```env
   DB_DSN=postgres://user:pass@localhost:5432/dbname?sslmode=disable
   ```
3. Run the API:
   ```bash
   go run main.go
   ```

### 2. Normal Cloudflare Worker
The code for the standard Cloudflare Worker with WASM is located in `code/cloudflare/my-first-worker`.

**Local Testing:**
1. Navigate to the project directory:
   ```bash
   cd code/cloudflare/my-first-worker
   ```
2. Install the dependencies:
   ```bash
   npm install
   ```
3. Run the development server (this will automatically build the Go code from the `api` folder into WASM):
   ```bash
   npm run dev
   ```

**Deploying to Cloudflare:**
```bash
npm run deploy
```

### 3. WASIP2 Test
The code for testing the native WASIP2 compilation is located in `code/wasip2test`.

**Local Testing:**
1. Navigate to the directory:
   ```bash
   cd code/wasip2test
   ```
2. Build the WASM binary using TinyGo (using the provided target file to match WIT definitions):
   ```bash
   tinygo build -target=wasip2-http.json -o main.wasm main.go
   ```
3. Serve the application using wasmtime (with CLI support enabled for sockets):
   ```bash
   wasmtime serve -Scli main.wasm
   ```

**Docker / Container execution:**
You can also build the provided scratch Dockerfile, provided your container runtime supports WASM (e.g. Docker/containerd with Wasmtime shim):
```bash
docker build -t wasip2test .
```

### 4. SpinKube / Fermyon Spin (Local & Kubernetes)
The code for this experiment is located in `code/spinkube/spingotest`.

**Local Testing:**
1. Make sure you have [Fermyon Spin](https://developer.fermyon.com/spin/v2/install) and [TinyGo](https://tinygo.org/getting-started/install/) installed.
2. Navigate to the spin app directory:
   ```bash
   cd code/spinkube/spingotest
   ```
3. Build the application:
   ```bash
   spin build
   ```
4. Run the application locally:
   ```bash
   spin up
   ```

**Local Kubernetes (Rancher Desktop):**
For a quick local Kubernetes setup with SpinKube support, you can use Rancher Desktop:

1. **Install Rancher Desktop**: Download it from the [official site](https://rancherdesktop.io/).
2. **Configure Container Engine**:
   - Open **Preferences** -> **Container Engine**.
   - Select **containerd**.
   - Ensure **Enable Wasm** is checked.
3. **Configure Kubernetes**:
   - Open **Preferences** -> **Kubernetes**.
   - Check **Enable Kubernetes**, **Enable Traefik**, and **Install Spin Operator**.
   - Apply changes and wait for the cluster to start.
4. **Set Context**:
   ```bash
   kubectl config use-context rancher-desktop
   ```
5. **Verify Installation**:
   - Check the **Cluster Dashboard** -> **Workloads**.
   - You should see `spin-operator-controller-manager` running in the `spin-operator` namespace.

**Deploying to Rancher Desktop:**
```bash
# Build and push to a temporary registry
spin build
spin registry push ttl.sh/wasm-test:latest

# Scaffold and apply
spin kube scaffold --from ttl.sh/wasm-test:latest | kubectl apply -f -

# Port forward to access the app
kubectl port-forward svc/wasm-test 8083:80
```

### 5. Kubernetes Deployment (General)
1. Ensure your Kubernetes cluster (e.g., K3s) has KEDA, SpinKube and the wasmtime runtime installed.
2. Apply the SpinApp custom resource:
   ```bash
   kubectl apply -f spinapp.yaml
   ```
3. Apply the deployment and ingress resources:
   ```bash
   kubectl apply -f deployment.yaml
   kubectl apply -f ingress.yaml
   ```

### 6. Cloudflare Containers Beta
The code for this experiment is located in `code/cf-container-test/withered-violet-31ce`.

**Local Testing:**
1. Navigate to the project directory:
   ```bash
   cd code/cf-container-test/withered-violet-31ce
   ```
2. Install the required Node.js dependencies:
   ```bash
   npm install
   ```
3. Run the development server:
   ```bash
   npm run dev
   ```
   Open `http://localhost:8787` to see the result.