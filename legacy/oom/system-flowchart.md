# Java堆内存转储自动化分析系统流程图

> 归档说明：本文件属于旧版 OOM 系统资料，与当前 go-watch-file 代码无直接关联，仅供参考。

## 系统整体流程图

```mermaid
graph TD
    A[Java应用OOM异常] --> B[生成hprof转储文件]
    B --> C[保存至/logs目录]
    C --> D[HostPath挂载到宿主机/data/logs]
    
    D --> E[DaemonSet监控工具]
    E --> F[Go fsnotify监听文件变化]
    F --> G{检测到新hprof文件?}
    
    G -->|是| H[自动上传至对象存储]
    G -->|否| F
    
    H --> I[解析文件路径]
    I --> J[提取namespace和deployment]
    J --> K[调用CMDB API查询开发人员]
    
    K --> L[获取应用关联信息]
    L --> M[生成通知消息]
    M --> N[推送至公司通知接口]
    
    H --> O[触发Jenkins工作流]
    O --> P[Jenkins启动MAT-CLI容器]
    P --> Q[容器下载hprof文件]
    Q --> R[生成HTML分析报告]
    R --> S[报告挂载到Nginx目录]
    
    S --> T[通过Nginx提供在线访问]
    T --> U[开发人员在线查看分析报告]
    
    N --> V[开发人员收到通知]
    V --> W[访问在线分析报告]
    
    H --> X[对象存储生命周期管理]
    X --> Y[15天后自动删除]
    
    %% AI分析流程
    R --> AI1[触发AI分析阶段]
    AI1 --> AI2[Jenkins启动AI分析容器]
    AI2 --> AI3[AI容器读取MAT分析报告]
    AI3 --> AI4[调用OpenAI API进行深度分析]
    AI4 --> AI5[生成AI智能分析报告]
    AI5 --> AI6[AI报告挂载到Nginx目录]
    AI6 --> AI7[更新index.html添加AI分析链接]
    
    AI7 --> T
    T --> AI8[开发人员查看AI智能分析]
    
    style A fill:#ff6b6b
    style E fill:#4ecdc4
    style H fill:#45b7d1
    style O fill:#96ceb4
    style P fill:#96ceb4
    style S fill:#feca57
    style Y fill:#ff9ff3
    style AI1 fill:#ff6b9d
    style AI2 fill:#ff6b9d
    style AI4 fill:#ff6b9d
    style AI5 fill:#ff6b9d
    style AI8 fill:#ff6b9d
```

## 详细工作流程图

```mermaid
flowchart TD
    Start([开始]) --> Monitor[监控/data/logs目录]
    Monitor --> Check{检测到新文件?}
    
    Check -->|否| Monitor
    Check -->|是| Validate{是hprof文件?}
    
    Validate -->|否| Monitor
    Validate -->|是| Upload[上传至对象存储]
    
    Upload --> Parse[解析文件路径]
    Parse --> Extract[提取应用信息]
    Extract --> Query[查询CMDB配置]
    
    Query --> Notify[推送通知给开发]
    Notify --> Trigger[触发Jenkins工作流]
    
    Trigger --> Analyze[Jenkins启动MAT-CLI容器]
    Analyze --> Download[容器下载hprof文件]
    Download --> Generate[生成HTML报告]
    Generate --> Deploy[报告挂载到Nginx]
    
    %% AI分析流程
    Generate --> AI_Trigger[触发AI分析阶段]
    AI_Trigger --> AI_Start[启动AI分析容器]
    AI_Start --> AI_Read[读取MAT分析报告]
    AI_Read --> AI_Process[调用OpenAI API分析]
    AI_Process --> AI_Generate[生成AI智能报告]
    AI_Generate --> AI_Deploy[AI报告部署到Nginx]
    AI_Deploy --> AI_Update[更新index.html链接]
    
    AI_Update --> Cleanup[清理临时文件]
    Deploy --> Cleanup
    Cleanup --> Monitor
    
    style Start fill:#e1f5fe
    style Monitor fill:#f3e5f5
    style Upload fill:#e8f5e8
    style Trigger fill:#fff3e0
    style Analyze fill:#fff3e0
    style Deploy fill:#fce4ec
    style AI_Trigger fill:#ffebee
    style AI_Start fill:#ffebee
    style AI_Process fill:#ffebee
    style AI_Generate fill:#ffebee
    style AI_Deploy fill:#ffebee
```

## 组件交互时序图

```mermaid
sequenceDiagram
    participant Java as Java应用
    participant Host as 宿主机
    participant DS as DaemonSet
    participant OSS as 对象存储
    participant CMDB as CMDB系统
    participant Notify as 通知服务
    participant Jenkins as Jenkins工作流
    participant MAT as MAT-CLI容器
    participant AI as AI分析容器
    participant OpenAI as OpenAI API
    participant Nginx as Nginx服务
    
    Java->>Host: OOM异常，生成hprof
    Host->>DS: 文件系统事件通知
    DS->>OSS: 上传hprof文件
    DS->>DS: 解析文件路径
    DS->>CMDB: 查询应用信息
    CMDB-->>DS: 返回开发人员信息
    DS->>Notify: 推送通知
    DS->>Jenkins: 触发工作流
    Jenkins->>MAT: 启动MAT-CLI容器
    MAT->>OSS: 下载hprof文件
    MAT->>MAT: 分析hprof文件
    MAT->>Nginx: 生成HTML报告
    
    %% AI分析时序
    MAT->>Jenkins: MAT分析完成
    Jenkins->>AI: 启动AI分析容器
    AI->>Nginx: 读取MAT分析报告
    AI->>OpenAI: 调用API进行深度分析
    OpenAI-->>AI: 返回AI分析结果
    AI->>Nginx: 生成AI智能报告
    AI->>Nginx: 更新index.html添加AI链接
    
    Notify->>开发: 发送通知消息
    开发->>Nginx: 访问在线报告
    开发->>Nginx: 查看AI智能分析
```

## AI分析详细流程图

```mermaid
flowchart TD
    AI_Start([AI分析开始]) --> AI_Read_HTML[读取MAT生成的HTML报告]
    AI_Read_HTML --> AI_Extract[提取关键信息]
    AI_Extract --> AI_Prepare[准备分析提示词]
    
    AI_Prepare --> AI_Call_API[调用OpenAI API]
    AI_Call_API --> AI_Process[处理API响应]
    AI_Process --> AI_Validate{分析结果有效?}
    
    AI_Validate -->|否| AI_Retry[重试分析]
    AI_Retry --> AI_Call_API
    
    AI_Validate -->|是| AI_Format[格式化HTML报告]
    AI_Format --> AI_Add_Style[添加CSS样式]
    AI_Add_Style --> AI_Add_Script[添加JavaScript交互]
    AI_Add_Script --> AI_Save[保存AI分析报告]
    
    AI_Save --> AI_Update_Index[更新index.html]
    AI_Update_Index --> AI_Complete([AI分析完成])
    
    style AI_Start fill:#e8f5e8
    style AI_Call_API fill:#ffebee
    style AI_Process fill:#ffebee
    style AI_Format fill:#e3f2fd
    style AI_Complete fill:#e8f5e8
```

## 数据流向图

```mermaid
graph LR
    subgraph "数据源"
        A[hprof文件]
        B[应用元数据]
        C[MAT分析报告]
    end
    
    subgraph "处理层"
        D[文件监控]
        E[路径解析]
        F[应用识别]
        G[MAT分析处理]
        H[AI分析处理]
    end
    
    subgraph "存储层"
        I[对象存储]
        J[MAT分析报告]
        K[AI分析报告]
    end
    
    subgraph "服务层"
        L[通知服务]
        M[在线报告]
        N[AI智能分析]
    end
    
    A --> D
    D --> E
    E --> F
    F --> B
    A --> I
    A --> G
    G --> J
    J --> H
    H --> K
    F --> L
    J --> M
    K --> N
    M --> N
    
    style A fill:#ffcdd2
    style C fill:#ffcdd2
    style D fill:#c8e6c9
    style G fill:#c8e6c9
    style H fill:#ffebee
    style I fill:#bbdefb
    style M fill:#fff9c4
    style N fill:#f3e5f5
```

## 部署架构图

```mermaid
graph TB
    subgraph "Kubernetes集群"
        subgraph "Node 1"
            A1[应用Pod1]
            A2[应用Pod2]
            M1[监控DaemonSet]
        end
        
        subgraph "Node 2"
            A3[应用Pod3]
            A4[应用Pod4]
            M2[监控DaemonSet]
        end
        
        subgraph "Node N"
            AN[应用PodN]
            MN[监控DaemonSet]
        end
    end
    
    subgraph "基础设施"
        H1[HostPath挂载]
        H2[HostPath挂载]
        H3[HostPath挂载]
    end
    
    subgraph "外部服务"
        OSS[对象存储]
        CMDB[CMDB系统]
        NOTIFY[通知服务]
        JENKINS[Jenkins工作流]
        NGINX[Nginx服务]
        OPENAI[OpenAI API]
    end
    
    subgraph "分析容器"
        MAT[MAT-CLI容器]
        AI[AI分析容器]
    end
    
    A1 --> H1
    A2 --> H1
    A3 --> H2
    A4 --> H2
    AN --> H3
    
    M1 --> H1
    M2 --> H2
    MN --> H3
    
    H1 --> OSS
    H2 --> OSS
    H3 --> OSS
    
    M1 --> CMDB
    M2 --> CMDB
    MN --> CMDB
    
    M1 --> NOTIFY
    M2 --> NOTIFY
    MN --> NOTIFY
    
    M1 --> JENKINS
    M2 --> JENKINS
    MN --> JENKINS
    
    JENKINS --> MAT
    JENKINS --> AI
    
    MAT --> NGINX
    AI --> NGINX
    AI --> OPENAI
    
    style OSS fill:#e3f2fd
    style CMDB fill:#f3e5f5
    style NOTIFY fill:#e8f5e8
    style JENKINS fill:#fff3e0
    style NGINX fill:#fff3e0
    style OPENAI fill:#ffebee
    style MAT fill:#e1f5fe
    style AI fill:#fce4ec
```

## Jenkins流水线流程图

```mermaid
flowchart TD
    J_Start([Jenkins流水线开始]) --> J_Params[接收参数]
    J_Params --> J_Stage1[Stage: dump]
    
    J_Stage1 --> J_Download[下载hprof文件]
    J_Download --> J_MAT[启动MAT-CLI容器]
    J_MAT --> J_Analyze[MAT分析处理]
    J_Analyze --> J_Report[生成HTML报告]
    J_Report --> J_Deploy[部署到Nginx]
    
    J_Deploy --> J_Stage2[Stage: ai-analysis]
    J_Stage2 --> J_AI_Start[启动AI分析容器]
    J_AI_Start --> J_AI_Read[读取MAT报告]
    J_AI_Read --> J_AI_Call[调用OpenAI API]
    J_AI_Call --> J_AI_Generate[生成AI报告]
    J_AI_Generate --> J_AI_Deploy[部署AI报告]
    J_AI_Deploy --> J_AI_Update[更新index.html]
    
    J_AI_Update --> J_Complete([流水线完成])
    
    style J_Start fill:#e8f5e8
    style J_Stage1 fill:#fff3e0
    style J_Stage2 fill:#ffebee
    style J_MAT fill:#e1f5fe
    style J_AI_Start fill:#fce4ec
    style J_AI_Call fill:#ffebee
    style J_Complete fill:#e8f5e8
```

## 监控指标图

```mermaid
graph TD
    subgraph "系统监控指标"
        M1[文件监控数量]
        M2[上传成功率]
        M3[处理延迟时间]
        M4[存储空间使用量]
    end
    
    subgraph "业务监控指标"
        B1[OOM发生频率]
        B2[文件大小分布]
        B3[开发响应时间]
        B4[在线报告访问量]
        B5[AI分析准确率]
    end
    
    subgraph "性能监控指标"
        P1[MAT分析耗时]
        P2[报告生成成功率]
        P3[通知推送延迟]
        P4[Jenkins工作流执行时间]
        P5[系统资源使用率]
        P6[AI分析耗时]
        P7[AI分析成功率]
        P8[OpenAI API调用延迟]
    end
    
    M1 --> Dashboard[监控仪表板]
    M2 --> Dashboard
    M3 --> Dashboard
    M4 --> Dashboard
    
    B1 --> Dashboard
    B2 --> Dashboard
    B3 --> Dashboard
    B4 --> Dashboard
    B5 --> Dashboard
    
    P1 --> Dashboard
    P2 --> Dashboard
    P3 --> Dashboard
    P4 --> Dashboard
    P5 --> Dashboard
    P6 --> Dashboard
    P7 --> Dashboard
    P8 --> Dashboard
    
    style Dashboard fill:#e8f5e8
    style M1 fill:#ffcdd2
    style B1 fill:#bbdefb
    style B5 fill:#ffebee
    style P1 fill:#fff9c4
    style P4 fill:#fff9c4
    style P6 fill:#fce4ec
    style P7 fill:#fce4ec
    style P8 fill:#fce4ec
```

## AI分析技术架构图

```mermaid
graph TB
    subgraph "AI分析模块"
        subgraph "输入层"
            A1[MAT HTML报告]
            A2[环境变量配置]
            A3[OpenAI API密钥]
        end
        
        subgraph "处理层"
            B1[HTML内容解析]
            B2[关键信息提取]
            B3[分析提示词构建]
            B4[OpenAI API调用]
            B5[响应结果处理]
        end
        
        subgraph "输出层"
            C1[AI分析报告生成]
            C2[HTML格式优化]
            C3[CSS样式添加]
            C4[JavaScript交互]
        end
        
        subgraph "部署层"
            D1[报告文件保存]
            D2[Nginx目录更新]
            D3[index.html链接更新]
        end
    end
    
    A1 --> B1
    A2 --> B3
    A3 --> B4
    B1 --> B2
    B2 --> B3
    B3 --> B4
    B4 --> B5
    B5 --> C1
    C1 --> C2
    C2 --> C3
    C3 --> C4
    C4 --> D1
    D1 --> D2
    D2 --> D3
    
    style A1 fill:#e3f2fd
    style B4 fill:#ffebee
    style C1 fill:#e8f5e8
    style D2 fill:#fff3e0
```

## 错误处理流程图

```mermaid
flowchart TD
    Error_Start([错误发生]) --> Error_Type{错误类型判断}
    
    Error_Type -->|文件监控错误| E1[检查DaemonSet状态]
    Error_Type -->|上传失败| E2[验证存储配置]
    Error_Type -->|MAT分析失败| E3[检查MAT工具状态]
    Error_Type -->|AI分析失败| E4[检查OpenAI API]
    Error_Type -->|通知推送失败| E5[检查通知服务]
    
    E1 --> Retry1[重启DaemonSet]
    E2 --> Retry2[重试上传]
    E3 --> Retry3[重新分析]
    E4 --> Retry4[重试AI分析]
    E5 --> Retry5[重试通知]
    
    Retry1 --> Success{重试成功?}
    Retry2 --> Success
    Retry3 --> Success
    Retry4 --> Success
    Retry5 --> Success
    
    Success -->|是| Complete([处理完成])
    Success -->|否| Alert[发送告警]
    Alert --> Manual[人工介入]
    Manual --> Complete
    
    style Error_Start fill:#ffcdd2
    style E4 fill:#ffebee
    style Retry4 fill:#fce4ec
    style Complete fill:#c8e6c9
    style Alert fill:#ffeb3b
```
