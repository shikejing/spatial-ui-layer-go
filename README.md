# spatial-ui-layer-go

面向扩展现实（XR）的空间界面布局与交互层。界面由若干面板（Panel）组成，
面板挂载在三维空间中的锚点上；头显与手柄姿态持续更新，用户通过凝视射线
与手柄射线选中面板。本库保证：在姿态持续更新与输入事件交错到达时，
布局与交互结果一致、可复现、可解释。

包路径：`github.com/shikejing/spatial-ui-layer-go/spatialui`

## 空间模型

- **坐标系**：右手系，米制单位。`Pose = Position(Vec3) + Orientation(Quat)`，
  表示刚体变换；`Pose.Mul` 组合父子变换，`Pose.Inverse` 求逆。
- **面板局部约定**：面板位于局部 `z=0` 平面，法线 `+Z` 朝向观察者，
  中心在原点，尺寸为 `Size{Width, Height}`。
- **锚点**（`AnchorKind`）：
  - `AnchorWorld`：世界锚定，位姿不随追踪变化；
  - `AnchorHead`：头部锚定，每帧取已提交的最新头部姿态；
  - `AnchorHand`：手柄锚定（`HandLeft` / `HandRight`）。
- **放置**：未分组面板的世界位姿 = `anchorPose * LocalOffset`；
  分组面板的世界位姿 = `groupAnchorPose * arrangedLocal * LocalOffset`，
  其中 `arrangedLocal` 由布局策略给出。
- **帧一致性**：每次 `Tick()` 先取一份 `PoseSnapshot`，本帧内所有面板的
  放置、两条射线的生成都使用该快照——同一帧内所有面板的相对关系自洽，
  不存在部分面板用新姿态、部分用旧姿态的情况。

## 布局

三种内置策略（`LayoutStrategy` 接口，输入按 `(Order, ID)` 稳定排序的面板）：

| 策略 | 说明 | 配置项 |
|---|---|---|
| `ArcLayout` | 沿水平圆弧排布，面板面向组锚点，居中面板在锚点正前方 | `Radius`、`AngleStep`（弧度）、`HeightStep` |
| `GridLayout` | 网格排布，从左到右、从上到下，每行水平居中 | `CellWidth`、`CellHeight`、`Columns` |
| `StackLayout` | 沿给定方向线性堆叠 | `Direction`、`Spacing` |

- **确定性**：排布次序为 `(Order, ID)`，与插入顺序无关——分批插入与
  一次性插入同一批面板得到完全相同的结果。
- **增量重算**：每个布局组缓存排布结果，仅在成员增删、尺寸变化
  （`Resize`）或策略变更（`SetLayout`）时重算该组；重算耗时与受影响
  组的大小相关，与面板总数无关。注意：每帧发布完整不可变快照本身是
  O(n) 的固有成本（放置变换、排序、命中检测）。

## 焦点语义

- **射线**：凝视射线 = 头部位姿前向（-Z）；手柄射线 = 右手柄位姿前向。
  两者取自同一姿态快照。
- **遮挡**：命中检测按射线距离取最近面板，更近的面板自然遮挡其后所有
  面板；距离相同按 `PanelID` 字典序裁决，结果可复现。
- **唯一焦点**：同一时刻最多一个面板持有焦点。两条射线命中不同面板时，
  手柄射线优先（主动输入优先）。
- **防抖滞回**：候选面板需连续命中 `SwitchFrames` 帧（默认 2）才获得
  焦点；焦点面板需连续丢失 `SwitchFrames` 帧才失去焦点。单帧抖动不会
  导致焦点在面板间反复跳变。
- **取消语义**：持有焦点的面板被 `RemovePanel` 移除时，下一帧快照的
  `Events` 中包含 `FocusCancelledRemoved`；被 `Remount` 重新挂载时包含
  `FocusCancelledRemount`。两者是可区分的取消结果，而非静默丢失。
  重挂后面板退出原布局组、按新锚点放置，焦点状态为"无焦点"——这是
  确定且可解释的状态；若面板重挂后仍在射线上，会按正常规则重新获得焦点。

## 姿态输入

- `SubmitPose(kind, sample)` 接受乱序、重复、抖动输入：仅当样本时间戳
  严格大于该路已提交时间戳时才接受，旧帧与重复帧被拒绝（幂等）。
- 提交的姿态在下次 `Tick()` 才进入已发布快照；任何读取方看到的布局与
  命中结果都对应某个已提交的姿态快照。

## 并发模型

- 所有写操作（`SubmitPose` / `Tick` / `AddPanel` / `RemovePanel` /
  `Remount` / `SetLayout` / `Resize`）由内部互斥锁串行化。
- `Snapshot()` 通过原子指针加载不可变快照：读取方永不阻塞写入，
  且永远看到自洽的单个快照；慢读取方持有的旧快照不会被后续帧修改。
- 内存有界：引擎只保留最新快照与每路最新姿态样本，不累积历史。

## 序列化

- `MarshalSnapshot` 输出带版本号的 JSON（当前 `SnapshotFormatVersion = 2`），
  面板按 ID 排序，同一快照的字节输出稳定，与产生它的帧严格对应。
- `UnmarshalSnapshot` 读取任意历史版本并迁移到当前版本：
  - **v1**：焦点为裸字符串、面板无 `Order`、无事件列表；迁移时补
    `Order=0`（退化为纯 ID 字典序，与 v1 行为一致）、焦点转结构化、
    事件列表为空。缺少 `version` 字段的数据按 v1 处理。
  - **v2**：当前格式。
  - 未知版本返回错误，不静默误读。

## API 速览

```go
e := spatialui.NewEngine()
e.SetLayout("menu", spatialui.ArcLayout{Radius: 2, AngleStep: 0.3},
    spatialui.AnchorHead, spatialui.HandLeft)
e.AddPanel(spatialui.Panel{ID: "p1", Group: "menu", Size: spatialui.Size{0.6, 0.4}})
e.SubmitPose(spatialui.TrackHead, spatialui.PoseSample{Pose: pose, Timestamp: ts})
snap := e.Tick()          // 推进一帧，返回不可变快照
snap = e.Snapshot()       // 任意 goroutine 可随时读取最近快照
data, _ := spatialui.MarshalSnapshot(snap)
```

## 测试

```sh
go test ./...          # 全部测试
go test -race ./...    # 含并发竞争检测
go test -bench=. ./... # 增量重算基准
```

覆盖场景：布局跟随（头部/世界/手柄锚定）、帧一致性、插入顺序无关的
确定性排布、遮挡命中、焦点防抖与取消、乱序/重复姿态、并发读取一致性、
运行期面板增删与重挂、序列化往返与 v1→v2 迁移。
