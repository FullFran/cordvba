"""uvicorn entrypoint: `uvicorn twin.main:app` or `python -m twin.main`."""

from twin.app import create_app

app = create_app()

if __name__ == "__main__":
    import uvicorn

    # Internal-only service: binds all interfaces inside its own container/network.
    uvicorn.run(app, host="0.0.0.0", port=8100)
