import type { Dictionary } from "../dictionary";
import type { Locale } from "../types";
import { en } from "./en";
import { es } from "./es";

export const DICTIONARIES: Record<Locale, Dictionary> = { es, en };
