# 准备隔离环境

本页用于选择兼容组件、安装并交接环境。只有任务要求验证功能时才继续[功能验证](validation.md)。需要完整命令与固定 values 的可复现起点时，使用[已测版本示例](preset-v2.10.0.md)；不要把该示例套到未核对的 Chart 或 K3s 版本上。

## 确认输入

开始前确认：

- 任务允许使用的主机、容器运行时和集群；是否允许创建新的隔离 k3d 集群。现有集群必须有明确归属和专用 kubeconfig/context。
- 主机架构、可用磁盘/内存、Docker、k3d、kubectl、Helm 版本，以及已有集群和 `nvidia.com/gpu` 资源提供者。不要覆盖已有集群或让两个 device-plugin 竞争同一资源。
- 用户指定的 HAMi、Mokka、K3s 版本及镜像来源。未指定时，可选择已测版本示例，但先确认示例镜像在当前架构和网络环境可用。
- 交付目标是环境就绪，还是某些具体功能的验证。仅准备环境时不运行测试工作负载。

若以上信息不足以安全选择或创建集群，先确认归属与范围，不猜测共享环境可用。

## 选择兼容版本

1. 选定 K3s 版本，记录实际 Kubernetes 和 containerd 版本；确认所选 Mokka 版本的 NRI 集成方式适用于该组合。v0.4.0 的说明见 [Mokka NRI 指南](https://github.com/NVIDIA/k8s-test-infra/blob/v0.4.0/docs/guides/node-wide-injection/README.md)。
2. 从[官方 Mokka 仓库](https://github.com/NVIDIA/k8s-test-infra)取同一版本的 OCI Chart 和镜像，固定镜像 digest。核对 Chart values、镜像架构和渲染结果；不要因拉取失败静默改用 `latest` 或非官方改造镜像。
3. 从目标 HAMi release/提交取 Chart 与匹配的应用镜像，核对 Chart `appVersion`、模板传入的启动参数和实际二进制。不要混用更新的 `master` Chart 与旧 release 镜像。
4. HAMi Chart 中的 kube-scheduler 镜像应匹配 K3s 的 Kubernetes 次版本；关闭 HAMi mock device-plugin，启用真实 NVIDIA device-plugin。配置 Mokka driver root、设备节点传递以及 agent 的 nodeSelector。字段名必须以选定 Chart 为准，不盲套示例 values。

如果无法确定 Chart、应用镜像或 NRI 的兼容关系，先做 `helm lint`、`helm template` 和镜像/参数检查；仍不确定时停止安装并报告缺口，不以 Pod Ready 代替兼容性证明。

## 安装与分层检查

在本任务专属 k3d/K3s 集群中使用唯一名称和单独 kubeconfig。所有 `kubectl`、`helm` 命令都显式指定该 kubeconfig/context，不切换默认 context。只在标记的 worker 上部署 Mokka；NRI overlay 可能影响同节点其他容器，因此不能放到共享节点。记录 Chart 包 SHA-256、渲染清单、values 副本及实际镜像 `imageID`。

按下列顺序安装与判断：

1. 安装 Mokka，并确认 agent、NRI Pod 就绪；检查 NRI 与 containerd 的连接日志、生成的模拟设备配置、driver root 和 `/dev/nvidia*`。Mokka 管理 Pod 的 `nvidia-smi` 可能显示 profile 原始设备数，以 agent 配置及设备节点为准。
2. 安装匹配版本的 HAMi。确认 scheduler、webhook、真实 NVIDIA device-plugin 和 monitor 启动，没有镜像拉取、参数或 ABI 错误。Mokka 的 NRI 默认可能排除 `kube-system`，不要把需要注入的 HAMi 组件放在被排除的 namespace。
3. 核对 Node 上不同的 GPU UUID、注册容量和实际驱动配置。容量应由模拟设备数和当前 `deviceSplitCount` 推导，不能照抄示例数值。此时只可报告「环境就绪／设备注册」，不能报告调度或 Allocate 通过。

失败时保留现场证据，优先检查镜像与架构、NRI socket、namespace 排除、nodeSelector、driver root、同名资源竞争、Chart 模板及 kube-scheduler 版本。修复后重新检查受影响的层级，不用关闭健康检查或更换未经核对的镜像制造成功结果。

## 交接与清理

交接集群名称、专用 kubeconfig **路径**和 context、版本/镜像 digest、两个 release 与 namespace、values 副本、Pod `imageID`、Node UUID/容量、日志异常、已验证和未验证层级。不要发送 kubeconfig 内容或其他凭据。默认保留已交接环境供后续任务使用。

只有用户要求销毁，或确认本任务创建的环境已结束且无人依赖时，才按确切名称清理：先保存证据，卸载本任务的 HAMi 与 Mokka release，等待相关 Pod 退出，再删除本任务的 k3d 集群和专用 kubeconfig。等待超时则保留现场排查；不删除未知资源、共享镜像或卷。
