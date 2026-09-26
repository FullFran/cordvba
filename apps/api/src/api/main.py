"""uvicorn entrypoint: `uvicorn api.main:app` or `python -m api.main`."""

from api.app import create_app

app = create_app()

if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
