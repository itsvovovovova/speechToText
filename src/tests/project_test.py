import pytest
import requests
import time

BASE_URL = "http://server:8080"

@pytest.fixture(scope="session")
def api_client():
    session = requests.Session()
    session.headers.update({'Content-Type': 'application/json'})
    yield session
    session.close()

@pytest.fixture(scope="session")
def unique_user():
    return {
        "username": f"test_{int(time.time())}",
        "password": "testpass1"
    }

def test_register_success(api_client):
    new_user = {
        "username": f"test_reg_{int(time.time())}",
        "password": "testpass1"
    }
    response = api_client.post(f"{BASE_URL}/register", json=new_user)
    assert response.status_code == 200
    assert response.json()["result"] == "ok"

def test_register_short_password(api_client):
    response = api_client.post(f"{BASE_URL}/register", json={
        "username": f"test_{int(time.time())}",
        "password": "short"
    })
    assert response.status_code == 400

@pytest.fixture(scope="session")
def registered_user(api_client, unique_user):
    response = api_client.post(f"{BASE_URL}/register", json=unique_user)
    assert response.status_code == 200
    return unique_user

@pytest.fixture(scope="session")
def auth_client(api_client, registered_user):
    response = api_client.post(f"{BASE_URL}/login", json=registered_user)
    assert response.status_code == 200
    data = response.json()
    assert data["result"] == "ok"
    assert "token" in data
    api_client.cookies.update(response.cookies)
    return api_client

def test_register_duplicate(api_client, registered_user):
    response = api_client.post(f"{BASE_URL}/register", json=registered_user)
    assert response.status_code == 400

def test_login_success(api_client, registered_user):
    response = api_client.post(f"{BASE_URL}/login", json=registered_user)
    assert response.status_code == 200
    data = response.json()
    assert data["result"] == "ok"
    assert "token" in data
    assert len(data["token"]) > 0

def test_login_wrong_pass(api_client, registered_user):
    wrong_data = {"username": registered_user["username"], "password": "wrongpass"}
    response = api_client.post(f"{BASE_URL}/login", json=wrong_data)
    assert response.status_code == 401

def test_login_nonexistent_user(api_client):
    response = api_client.post(f"{BASE_URL}/login", json={
        "username": f"nonexistent_{int(time.time())}",
        "password": "testpass1"
    })
    assert response.status_code == 401

def test_protected_endpoints(auth_client):
    audio_data = {"audio": "https://static.deepgram.com/examples/Bueller-Life-moves-pretty-fast.wav"}
    audio_response = auth_client.post(f"{BASE_URL}/audio", json=audio_data)
    assert audio_response.status_code == 202

    task_id = audio_response.json()["task_id"]
    assert task_id != ""


    status_response = auth_client.get(f"{BASE_URL}/status", params={"task_id": task_id})
    assert status_response.status_code == 200

    result_response = auth_client.get(f"{BASE_URL}/result", params={"task_id": task_id})
    assert result_response.status_code == 200

def test_invalid_audio_url(auth_client):
    response = auth_client.post(f"{BASE_URL}/audio", json={"audio": "not-a-url"})
    assert response.status_code == 400

def test_delete_task(auth_client):
    audio_data = {"audio": "https://static.deepgram.com/examples/Bueller-Life-moves-pretty-fast.wav"}
    audio_response = auth_client.post(f"{BASE_URL}/audio", json=audio_data)
    assert audio_response.status_code == 202

    task_id = audio_response.json()["task_id"]
    delete_response = auth_client.delete(f"{BASE_URL}/tasks/{task_id}")
    assert delete_response.status_code == 200

def test_logout(api_client, registered_user):
    response = api_client.post(f"{BASE_URL}/login", json=registered_user)
    assert response.status_code == 200
    session = requests.Session()
    session.cookies.update(response.cookies)
    logout_response = session.post(f"{BASE_URL}/logout")
    assert logout_response.status_code == 200

def test_unauthorized_access(api_client):
    api_client.cookies.clear()

    endpoints = [
        ("GET", "/status"),
        ("GET", "/result"),
        ("POST", "/audio", {"audio": "https://example.com/audio.wav"})
    ]

    for method, path, *data in endpoints:
        if method == "GET":
            response = api_client.get(f"{BASE_URL}{path}")
        else:
            response = api_client.post(f"{BASE_URL}{path}", json=data[0])

        assert response.status_code == 401, f"Should be unauthorized for {method} {path}"
