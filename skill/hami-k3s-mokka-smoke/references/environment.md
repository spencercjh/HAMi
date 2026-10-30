# 准备隔离环境

本页用于选择兼容组件、安装并交接环境。只有任务要求验证功能时才继续[功能验证](validation.md)。[固定版本示例](preset-v2.10.0.md)仅用于复现已测环境；不要把它当成默认版本，也不要将其 values 套到未核对的 Chart 或 K3s 版本上。

## 确认输入

开始前确认：

- 任务允许使用的主机、容器运行时和集群；是否允许创建新的隔离 k3d 集群。现有集群必须有明确归属和专用 kubeconfig/context。
- 主机架构、可用磁盘/内存、Docker、k3d、kubectl、Helm 版本，以及已有集群和 `nvidia.com/gpu` 资源提供者。不要覆盖已有集群或让两个 device-plugin 竞争同一资源。
- 用户指定的 HAMi、Mokka、K3s 版本及镜像来源。未指定 HAMi 或 Mokka 版本时，在执行时分别查询最新的非预发布正式版；不要默认使用本 Skill 编写时的版本。
- 交付目标是环境就绪，还是某些具体功能的验证。仅准备环境时不运行测试工作负载。

若以上信息不足以安全选择或创建集群，先确认归属与范围，不猜测共享环境可用。

## 选择兼容版本

1. 用户未指定版本时，在执行时分别查询 [HAMi 最新正式 release](https://github.com/Project-HAMi/HAMi/releases/latest) 和 [Mokka 最新正式 release](https://github.com/NVIDIA/k8s-test-infra/releases/latest)，记录 tag、发布时间及对应提交。GitHub `releases/latest` 排除草稿与预发布；也可用 `gh api repos/Project-HAMi/HAMi/releases/latest` 和 `gh api repos/NVIDIA/k8s-test-infra/releases/latest` 查询。不要从 `master`、`latest` 镜像标签或本 Skill 的示例推断当前正式版。HAMi v2.10.0 是已测基线；选用后续版本仍须核验，不表示所有后续版本已通过测试。
2. 选定 K3s 版本，记录实际 Kubernetes 和 containerd 版本；阅读所选 Mokka release 的安装说明与破坏性变更，确认 NRI 和容器运行时集成适用于该组合。再从官方 Mokka release 取同版本 OCI Chart 与镜像，固定镜像 digest，核对 Chart `appVersion`、values schema、镜像架构和渲染结果。
3. 从所选 HAMi release 取 Chart 与匹配的应用镜像，核对 Chart `appVersion`、模板传入的启动参数和实际二进制。不要混用更新的 `master` Chart 与旧 release 镜像。
4. HAMi Chart 中的 kube-scheduler 镜像应匹配 K3s 的 Kubernetes 次版本；关闭 HAMi mock device-plugin，启用真实 NVIDIA device-plugin。配置 Mokka driver root、设备节点传递以及 agent 的 nodeSelector。若所选 Mokka 版本的 NRI 只向获 GPU 分配或显式声明的 Pod 注入驱动，还须检查 HAMi device-plugin Pod 是否需要注入注解。若 Mokka 会自动修改容器运行时，先确认该功能适用于所选 K3s 节点。字段名必须以选定 Chart 为准，不盲套示例 values。

若最新正式版的 Chart、镜像或 K3s 组合不兼容，先记录具体失败证据；只有用户要求复现或同意改用较早版本时，才选较早正式版，并说明偏离最新版本的原因。不要因拉取失败静默改用 `latest` 标签或非官方改造镜像。

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
