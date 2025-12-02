# Go Envir

PHP Envoy 스타일의 Go SSH 배포 도구

## 설치

```bash
go install github.com/yejune/go-envir@latest
```

또는 소스에서 빌드:

```bash
git clone https://github.com/yejune/go-envir.git
cd go-envir
go install .
```

## 빠른 시작

```bash
# 1. 프로젝트 폴더에서 초기화
envir init

# 2. Envirfile.yaml 수정 (서버 정보, 태스크 설정)

# 3. 배포 실행
envir deploy
```

## 명령어

| 명령어 | 설명 |
|-------|------|
| `envir` | 사용 가능한 태스크 목록 표시 (Envirfile.yaml 있을 때) |
| `envir init` | Envirfile.yaml 템플릿 생성 |
| `envir list` | 사용 가능한 태스크 목록 |
| `envir <task>` | 태스크 실행 |
| `envir <task> --on=<server>` | 특정 서버에서만 실행 |
| `envir help` | 도움말 |

## Envirfile.yaml 구조

```yaml
# 서버 정의
servers:
  production:
    host: example.com
    user: ubuntu
    key: ~/.ssh/id_rsa    # 기본값: ~/.ssh/id_rsa
    port: 22              # 기본값: 22

  staging:
    host: staging.example.com
    user: deploy

# 태스크 정의
tasks:
  deploy:
    description: "프로덕션 배포"
    on: [production]      # 실행할 서버 (생략 시 첫 번째 서버)
    scripts:
      - local: echo "로컬에서 빌드"
      - upload: ./app:/remote/path/app
      - run: sudo systemctl restart myapp

  logs:
    description: "로그 확인"
    on: [production]
    scripts:
      - run: sudo journalctl -u myapp -f
```

## 스크립트 타입

### local - 로컬 명령 실행

```yaml
scripts:
  - local: GOOS=linux GOARCH=amd64 go build -o server .
  - local: npm run build
```

### upload - 파일 업로드 (SCP)

```yaml
scripts:
  - upload: ./server:/app/server-new
  - upload: ./dist:/var/www/html
```

### run - 원격 명령 실행

```yaml
scripts:
  - run: sudo systemctl restart myapp
  - run: |
      cd /app
      ./migrate.sh
      sudo systemctl restart myapp
```

## 예제: Go 웹 서버 배포

```yaml
servers:
  production:
    host: myserver.com
    user: ubuntu
    key: ~/.ssh/id_rsa

tasks:
  deploy:
    description: "프로덕션 배포"
    on: [production]
    scripts:
      # 1. 로컬에서 Linux용 빌드
      - local: GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server-linux .

      # 2. 서버로 업로드
      - upload: server-linux:/app/server-new

      # 3. 서버에서 교체 및 재시작
      - run: |
          cd /app
          mv server server-old 2>/dev/null || true
          mv server-new server
          chmod +x server
          sudo systemctl restart myapp

      # 4. 로컬 빌드 파일 삭제
      - local: rm -f server-linux

  status:
    description: "서비스 상태 확인"
    on: [production]
    scripts:
      - run: sudo systemctl status myapp --no-pager

  logs:
    description: "실시간 로그"
    on: [production]
    scripts:
      - run: sudo journalctl -u myapp -f

  rollback:
    description: "이전 버전으로 롤백"
    on: [production]
    scripts:
      - run: |
          cd /app
          mv server server-failed
          mv server-old server
          sudo systemctl restart myapp
```

## 환경 변수

Envirfile.yaml에서 환경 변수 사용 가능:

```yaml
servers:
  production:
    host: $DEPLOY_HOST
    user: $DEPLOY_USER
    key: $SSH_KEY_PATH
```

## 여러 서버에 배포

### 순차 실행 (기본)

```yaml
tasks:
  deploy:
    on: [web1, web2]  # 순차적으로 실행
    scripts:
      - upload: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

### 병렬 실행

```yaml
tasks:
  deploy:
    on: [web1, web2, web3]
    parallel: true  # 모든 서버에 동시 실행
    scripts:
      - upload: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

병렬 실행 시:
- 모든 서버에 동시에 배포
- 각 서버의 출력은 버퍼링 후 순서대로 표시
- 하나라도 실패하면 에러 반환

### 특정 서버만 지정

```bash
envir deploy --on=web1
```

## 라이선스

MIT
