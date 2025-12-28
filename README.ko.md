# Gorelay

PHP Envoy 스타일의 Go SSH 배포 도구

## 설치

```bash
go install github.com/yejune/gorelay@latest
```

또는 소스에서 빌드:

```bash
git clone https://github.com/yejune/gorelay.git
cd gorelay
go install .
```

## 빠른 시작

```bash
# 1. 프로젝트 폴더에서 초기화
gorelay init

# 2. Gorelayfile.yaml 수정 (서버 정보, 태스크 설정)

# 3. 배포 실행
gorelay deploy
```

## 명령어

| 명령어 | 설명 |
|-------|------|
| `gorelay` | 사용 가능한 태스크 목록 표시 (Gorelayfile.yaml 있을 때) |
| `gorelay init` | Gorelayfile.yaml 템플릿 생성 |
| `gorelay list` | 사용 가능한 태스크 목록 |
| `gorelay <task>` | 태스크 실행 |
| `gorelay <task> --on=<server>` | 특정 서버에서만 실행 |
| `gorelay <task> -v` | 상세 출력으로 실행 |
| `gorelay help` | 도움말 |

## Gorelayfile.yaml 구조

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
      - sync: ./app:/remote/path/app
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
  - local: npm run build
```

### sync - 체크섬 비교 업로드 (변경분만)

```yaml
scripts:
  - sync: ./server:/app/server
  - sync: ./dist:/var/www/html
```

특징:
- 업로드 전 SHA256 체크섬 비교
- 변경된 파일만 업로드
- 업로드 후 무결성 검증

### tar - tar.gz 압축 업로드 (원자적)

```yaml
scripts:
  - tar: ./server:/app/server
  - tar: ./dist:/var/www/html
```

특징:
- 메모리에서 tar.gz 생성
- 원격 서버에서 압축 해제
- 원자적: 전부 아니면 전무 (부분 업로드 없음)
- 프로덕션 배포에 적합

### scp - 직접 업로드 (체크섬 없음)

```yaml
scripts:
  - scp: ./server:/app/server
  - scp: ./dist:/var/www/html
```

특징:
- 가장 빠른 방식 (체크섬 비교 없음)
- 모든 파일 무조건 업로드
- 빠른 개발 배포에 적합

### run - 원격 명령 실행

```yaml
scripts:
  - run: echo "서버에서 실행"
  - run: |
      cd /app
      ./server restart
      sudo systemctl restart myapp
```

## 업로드 방식 비교

| 방식 | 체크섬 | 원자적 | 속도 | 용도 |
|------|--------|--------|------|------|
| `sync` | ✓ (전후) | ✗ | 중간 | 점진적 배포 |
| `tar` | ✓ (tar 내용) | ✓ | 중간 | 프로덕션 배포 |
| `scp` | ✗ | ✗ | 빠름 | 개발 배포 |

## 예제: Go 웹 서버 배포

```yaml
servers:
  production:
    host: example.com
    user: ubuntu
    key: ~/.ssh/id_rsa

tasks:
  deploy:
    description: "프로덕션 배포"
    on: [production]
    scripts:
      # 1. 로컬에서 Linux용 빌드
      - local: GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server-linux .

      # 2. tar로 업로드 (원자적)
      - tar: server-linux:/app/server-new

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

Gorelayfile.yaml에서 환경 변수 사용 가능:

```yaml
servers:
  production:
    host: $DEPLOY_HOST
    user: $DEPLOY_USER
    key: $SSH_KEY_PATH
```

## 여러 서버에 배포

### 배열 호스트

```yaml
servers:
  web:
    hosts:
      - web1.example.com
      - web2.example.com
      - web3.example.com
    user: ubuntu

tasks:
  deploy:
    on: [web]  # 자동으로 web[0], web[1], web[2]로 확장
    parallel: true
    scripts:
      - tar: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

### 순차 실행 (기본)

```yaml
tasks:
  deploy:
    on: [web1, web2]  # 순차적으로 실행
    scripts:
      - tar: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

### 병렬 실행

```yaml
tasks:
  deploy:
    on: [web1, web2, web3]
    parallel: true  # 모든 서버에 동시 실행
    scripts:
      - tar: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

병렬 실행 시:
- 모든 서버에 동시에 배포
- 각 서버의 출력은 버퍼링 후 순서대로 표시
- 하나라도 실패하면 에러 반환

### 특정 서버만 지정

```bash
gorelay deploy --on=web1
```

## 로깅

Gorelayfile.yaml에서 파일 로깅 활성화:

```yaml
log:
  enabled: true
  path: ./gorelay.log  # 기본값: gorelay.log
```

활성화하면 모든 출력이 타임스탬프와 함께 로그 파일에 저장됩니다.

## 상세 모드

`-v` 플래그로 상세 출력:

```bash
gorelay deploy -v
```

표시 내용:
- 업로드 파일의 SHA256 체크섬
- 각 단계별 소요 시간
- 총 실행 시간

## 라이선스

MIT
