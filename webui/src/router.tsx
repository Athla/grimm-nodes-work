
import { createBrowserRouter } from "react-router-dom";
import Home from "./Home";
// Define your routes
const router = createBrowserRouter([
  {
    path: "/",
    element: <Home />,
  },
]);

export default router;

  