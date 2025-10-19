# 多阶段构建 Dockerfile
# 第一阶段：构建阶段
FROM maven:3.6.3-openjdk-17 AS builder

# 设置工作目录
WORKDIR /app

# 复制 pom.xml 和源代码
COPY pom.xml .
COPY src ./src

# 构建应用
RUN mvn clean package -DskipTests

# 第二阶段：运行阶段
FROM openjdk:17-bullseye AS runtime

# 设置工作目录
WORKDIR /app

# 安装必要的工具和时区配置
RUN apt-get update && \
    apt-get install -y curl tzdata tini && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 设置时区为上海
ENV TZ=Asia/Shanghai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# 创建必要的目录
RUN mkdir -p /app/logs /app/dumps

# 从构建阶段复制 JAR 文件
COPY --from=builder /app/target/oom-test-1.0.0.jar app.jar

# 设置目录权限
RUN chmod -R 755 /app

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8080/actuator/health || exit 1

# 使用 tini 作为 init 进程，使用 shell 形式的 CMD 支持变量替换
ENTRYPOINT ["/usr/bin/tini", "-s", "--"]
CMD java -Xms64m -Xmx128m -XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="/app/dumps/oom-test-app-$(hostname)-$(date '+%s').hprof" -Xlog:gc*:file=/app/logs/gc.log:time,uptime:filecount=5,filesize=20M -XX:+UseG1GC -XX:MaxGCPauseMillis=200 -XX:+UseStringDeduplication -XX:+UseCompressedClassPointers -XX:+UseCompressedOops -Djava.security.egd=file:/dev/./urandom -Dspring.profiles.active=docker -Duser.timezone=Asia/Shanghai -jar app.jar
